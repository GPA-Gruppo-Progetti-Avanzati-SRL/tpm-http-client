package restclient

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-common/util"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/har"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/hartracing"
	"github.com/go-resty/resty/v2"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/rs/zerolog/log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type LinkedService struct {
	Cfg *Config
}

func NewInstanceWithConfig(cfg *Config) (*LinkedService, error) {
	lks := &LinkedService{Cfg: cfg}
	return lks, nil
}

func (lks LinkedService) NewClient(opts ...Option) (*Client, error) {
	cli := NewClient(lks.Cfg, opts...)
	return cli, nil
}

type Client struct {
	cfg Config

	restClient *resty.Client
	span       opentracing.Span
	spanOwned  bool

	harSpan hartracing.Span
}

func NewClient(cfg *Config, opts ...Option) *Client {

	const semLogContext = "http-client::new"

	var clientOptions Config
	if cfg == nil {
		clientOptions = Config{TraceRequestName: "rest-client"}
	} else {
		clientOptions = *cfg
	}

	for _, o := range opts {
		o(&clientOptions)
	}

	s := &Client{
		cfg:     clientOptions,
		span:    clientOptions.Span,
		harSpan: clientOptions.HarSpan,
	}

	if clientOptions.TraceGroupName != "" {
		s.span = s.startSpan(clientOptions.Span, nil, clientOptions.TraceGroupName)
		s.spanOwned = true
	}

	s.restClient = resty.New()
	if s.cfg.RestTimeout != 0 {
		s.restClient.SetTimeout(s.cfg.RestTimeout)
		log.Trace().Dur("rest-timeout", s.cfg.RestTimeout).Msg(semLogContext)
	}

	if s.cfg.RetryCount != 0 {
		s.restClient.SetRetryCount(s.cfg.RetryCount)
		log.Trace().Int("rest-retry-count", s.cfg.RetryCount).Msg(semLogContext)
	}

	if s.cfg.RetryWaitTime != 0 {
		s.restClient.SetRetryWaitTime(s.cfg.RetryWaitTime)
		log.Trace().Dur("rest-wait-time", s.cfg.RetryWaitTime).Msg(semLogContext)
	}

	if s.cfg.RetryMaxWaitTime != 0 {
		s.restClient.SetRetryMaxWaitTime(s.cfg.RetryMaxWaitTime)
		log.Trace().Dur("rest-max-wait-time", s.cfg.RetryMaxWaitTime).Msg(semLogContext)
	}

	if len(s.cfg.RetryOnHttpError) > 0 {
		s.restClient.AddRetryCondition(retryCondition(s.cfg.RetryOnHttpError))
		log.Trace().Interface("rest-retry on error", s.cfg.RetryOnHttpError).Msg(semLogContext)
	}

	if s.cfg.SkipVerify {
		s.restClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return s
}

func retryCondition(errorsList []int) resty.RetryConditionFunc {
	return func(resp *resty.Response, err error) bool {

		const semLogContext = "http-client::retry-condition-func"

		if len(errorsList) == 0 || err != nil {
			log.Trace().Err(err).Msg(semLogContext + " retry condition satisifed")
			return true
		}

		sc := resp.StatusCode()
		for i := 0; i < len(errorsList); i++ {
			if sc == errorsList[i] {
				log.Trace().Int("http-status", sc).Msg(semLogContext + " retry condition satisfied")
				return true
			}
		}

		log.Trace().Int("http-status", sc).Msg(semLogContext + " retry condition NOT satisfied")
		return false
	}
}

func (s *Client) Close() {
	if s.span != nil && s.spanOwned {
		s.span.Finish()
	}
}

func (s *Client) NewRequest(method string, url string, body []byte, headers har.NameValuePairs, params har.NameValuePairs) (*har.Request, error) {

	var hs []har.NameValuePair

	ct := ""

	// Setting first the default headers... in principle I should avoid dups....
	for _, h := range s.cfg.Headers {
		hs = append(hs, har.NameValuePair{Name: h.Name, Value: h.Value})
		if strings.ToLower(h.Name) == "content-type" {
			ct = h.Value
		}
	}

	for _, h := range headers {
		hs = append(hs, har.NameValuePair{Name: h.Name, Value: h.Value})
		if strings.ToLower(h.Name) == "content-type" {
			ct = h.Value
		}
	}

	/*
		Signature was http.Headers
		for n, h := range headers {
			for i := range h {
				hs = append(hs, NameValuePair{Name: n, Value: h[i]})
				if strings.ToLower(n) == "content-type" {
					ct = h[i]
				}
			}
		}
	*/

	pars := make([]har.Param, 0)
	for _, h := range params {
		pars = append(pars, har.Param{Name: h.Name, Value: h.Value})
	}

	bodySize := -1
	var postData *har.PostData
	if len(body) > 0 {
		if ct == "" {
			// Default content-type used if something else is not found.
			ct = "application/json"
		}

		postData = &har.PostData{
			MimeType: ct,
			Data:     body,
			Params:   pars,
		}
	}

	req := &har.Request{
		Method:      method,
		URL:         url,
		HTTPVersion: "1.1",
		Headers:     hs,
		HeadersSize: -1,
		Cookies:     []har.Cookie{},
		QueryString: []har.NameValuePair{},
		BodySize:    int64(bodySize),
		PostData:    postData,
	}

	return req, nil
}

func (s *Client) Execute(reqDef *har.Request, execOpts ...ExecutionContextOption) (*har.Entry, error) {

	const semLogContext = "http-client::execute"

	execCtx := ExecutionContext{}
	for _, o := range execOpts {
		o(&execCtx)
	}

	now := time.Now()
	e := &har.Entry{
		Comment:         execCtx.RequestId,
		StartedDateTime: now.Format("2006-01-02T15:04:05.999999999Z07:00"),
		StartDateTimeTm: now,
		Request:         reqDef,
	}

	var reqSpanName string
	if s.cfg.TraceRequestName != "" {
		reqSpanName = strings.Replace(s.cfg.TraceRequestName, RequestTraceNameOpNamePlaceHolder, execCtx.OpName, 1)
		reqSpanName = strings.Replace(reqSpanName, RequestTraceNameRequestIdPlaceHolder, execCtx.RequestId, 1)
	} else {
		reqSpanName = strings.Join([]string{execCtx.OpName, execCtx.RequestId}, "_")
	}

	reqSpan := s.startSpan(s.span, execCtx.Span, reqSpanName)
	defer reqSpan.Finish()

	// create a har-span and set a tag in the opentracing span.... if hartracing has been enabled...
	var harSpan hartracing.Span
	if s.cfg.IsHarTracingEnabled() {
		harSpan = s.startHarSpan(s.harSpan, execCtx.HarSpan)
		defer harSpan.Finish()
		reqSpan.SetTag(hartracing.HARTraceOpenTracingTagName, harSpan.Id())
	}

	// reqDef.Headers = append(reqDef.Headers, NameValuePair{Name: "Accept", Value: "application/json"})
	req := s.getRequestWithSpans(reqDef, reqSpan, harSpan)

	var resp *resty.Response
	var err error

	u := reqDef.URL
	switch reqDef.Method {
	case http.MethodGet:
		resp, err = req.Get(u)
	case http.MethodHead:
		resp, err = req.Head(u)
	case http.MethodPost:
		resp, err = req.Post(u)
	case http.MethodPut:
		resp, err = req.Put(u)
	case http.MethodDelete:
		resp, err = req.Delete(u)
	}

	var sc int
	var st string
	if resp != nil {
		sc = resp.StatusCode()
		st = resp.Status()
	}

	var r *har.Response
	if err == nil {

		r = &har.Response{
			Status:      sc,
			HTTPVersion: "1.1",
			StatusText:  st,
			HeadersSize: -1,
			BodySize:    resp.Size(),
			Cookies:     []har.Cookie{},
			Content: &har.Content{
				MimeType: resp.Header().Get("Content-type"),
				Size:     resp.Size(),
				Data:     resp.Body(),
			},
		}

		for n, _ := range resp.Header() {
			r.Headers = append(r.Headers, har.NameValuePair{Name: n, Value: resp.Header().Get(n)})
		}
	} else {
		if resp != nil {
			log.Warn().Msg(semLogContext + " error is not nil but response is present... compare to symphony behaviour.. v0.0.15")
		}
		sc, st = DetectStatusCodeStatusTextFromError(sc, err)
		err = util.NewError(strconv.Itoa(sc), err)
		r = har.NewResponse(sc, st, "text/plain", []byte(err.Error()), nil)
	}

	s.setSpanTags(reqSpan, execCtx.OpName, execCtx.RequestId, execCtx.LRAId, u, reqDef.Method, sc, err)

	if e.StartedDateTime != "" {
		elapsed := time.Since(e.StartDateTimeTm)
		e.Time = float64(elapsed.Milliseconds())
	}

	e.Timings = &har.Timings{
		Blocked: -1,
		DNS:     -1,
		Connect: -1,
		Send:    -1,
		Wait:    e.Time,
		Receive: -1,
		Ssl:     -1,
	}

	e.Response = r

	if harSpan != nil {
		harSpan.AddEntry(e)
	}

	return e, err
	// return resp.StatusCode(), resp.Body(), resp.Header(), err
}

func (s *Client) getRequestWithSpans(reqDef *har.Request, reqSpan opentracing.Span, reqHarSpan hartracing.Span) *resty.Request {

	req := s.restClient.R()
	// Transmit the span's TraceContext as HTTP headers on our outbound request.
	_ = opentracing.GlobalTracer().Inject(reqSpan.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
	_ = hartracing.GlobalTracer().Inject(reqHarSpan.Context(), hartracing.HTTPHeadersCarrier(req.Header))

	switch reqDef.Method {
	case http.MethodGet:
	case http.MethodHead:
	case http.MethodDelete:
	case http.MethodPost:
		if reqDef.HasBody() {
			req = req.SetBody(reqDef.PostData.Data)
		}

	case http.MethodPut:
		if reqDef.HasBody() {
			req = req.SetBody(reqDef.PostData.Data)
		}
	}

	// Setting more specific headers next
	for _, h := range reqDef.Headers {
		req.SetHeader(h.Name, h.Value)
	}

	return req
}

/*
func (s *Client) getRequestSpan() opentracing.Span {

	var reqSpan opentracing.Span

	if s.span != nil {
		parentCtx := s.span.Context()
		reqSpan = opentracing.StartSpan(
			s.cfg.TraceOpName,
			opentracing.ChildOf(parentCtx),
		)
	} else {
		reqSpan = opentracing.StartSpan(
			s.cfg.TraceOpName,
		)
	}

	return reqSpan
}
*/

func (s *Client) startHarSpan(clientSpan hartracing.Span, requestSpan hartracing.Span) hartracing.Span {

	const semLogContext = "http-client::start-har-span"
	var span hartracing.Span

	parentSpan := requestSpan
	if parentSpan == nil {
		parentSpan = clientSpan
	}

	if parentSpan != nil {
		span = hartracing.GlobalTracer().StartSpan(hartracing.ChildOf(parentSpan.Context()))
		log.Trace().Str("span-id", span.Id()).Str("parent-span-id", parentSpan.Id()).Msg(semLogContext + " - started a child span")
	} else {
		span = hartracing.GlobalTracer().StartSpan()
		log.Trace().Str("span-id", span.Id()).Msg(semLogContext + " - starting a brand new span")
	}

	return span
}

func (s *Client) startSpan(groupParentSpan, requestParentSpan opentracing.Span, spanName string) opentracing.Span {

	const semLogContext = "http-client::start-span"
	var span opentracing.Span

	parentSpan := groupParentSpan
	if requestParentSpan != nil {
		parentSpan = requestParentSpan
		if groupParentSpan != nil {
			log.Warn().Str("trace-group-name", s.cfg.TraceGroupName).Msg(semLogContext + " configuration issue.... a parent span has been set on the request but a group-span is also present")
		}
	}

	if parentSpan != nil {
		parentCtx := parentSpan.Context()
		span = opentracing.StartSpan(
			spanName,
			opentracing.ChildOf(parentCtx),
		)
	} else {
		span = opentracing.StartSpan(
			spanName,
		)
	}

	return span
}

func (s *Client) setSpanTags(reqSpan opentracing.Span, opName, reqId, lraId, endpoint, method string, statusCode int, err error) {

	reqSpan.SetTag(util.HttpUrlTraceTag, endpoint)
	reqSpan.SetTag(util.HttpMethodTraceTag, method)
	reqSpan.SetTag(util.HttStatusCodeTraceTag, statusCode)

	if opName != "" {
		reqSpan.SetTag(OpNameTraceTag, opName)
	}

	if lraId != "" {
		reqSpan.SetTag(LraHttpContextTraceTag, lraId)
	}

	if reqId != "" {
		reqSpan.SetTag(RequestIdTraceTag, reqId)
	}

	if err != nil {
		reqSpan.SetTag("error", err.Error())
		ext.Error.Set(reqSpan, true)
	}
}

func DetectStatusCodeStatusTextFromError(c int, err error) (int, string) {
	if c != 0 {
		return c, http.StatusText(c)
	}

	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return http.StatusRequestTimeout, http.StatusText(http.StatusRequestTimeout)
	}

	rc := http.StatusInternalServerError
	rt := http.StatusText(rc)

	switch t := err.(type) {
	case *url.Error:
		rc = http.StatusServiceUnavailable
		rt = http.StatusText(rc)
		if t1, ok := t.Err.(*net.OpError); ok {
			switch t1.Op {
			case "dial":
				rt = "Unknown host"
			case "read":
				rt = "Connection refused"
			}
		}

	case *net.OpError:
		rc = http.StatusServiceUnavailable
		rt = http.StatusText(rc)
		if errors.Is(t, syscall.ECONNRESET) {
			rt = "Reset by peer"
		} else {
			switch t.Op {
			case "dial":
				rt = "Unknown host"
			case "read":
				rt = "Connection refused"
			}
		}

	case syscall.Errno:
		rc = http.StatusServiceUnavailable
		rt = http.StatusText(rc)
		if t == syscall.ECONNREFUSED {
			rt = "Connection refused"
		}
	}

	return rc, rt
}
