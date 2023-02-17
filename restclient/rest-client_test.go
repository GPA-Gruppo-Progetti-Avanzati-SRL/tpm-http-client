package restclient_test

import (
	"encoding/json"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/har"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/hartracing"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/hartracing/filetracer"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-client/restclient"
	"github.com/opentracing/opentracing-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	ServerPort = 9090
)

func TestRestClient(t *testing.T) {

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	harCloser, err := InitHarTracing(t)
	require.NoError(t, err)
	if harCloser != nil {
		defer harCloser.Close()
	}

	closer, err := InitTracing(t)
	require.NoError(t, err)
	if closer != nil {
		defer closer.Close()
	}

	cfg := restclient.Config{
		RestTimeout:      15 * time.Second,
		SkipVerify:       true,
		Headers:          []restclient.Header{{Name: "x-api-key", Value: "pippo"}},
		TraceGroupName:   "rest-client",
		TraceRequestName: "rest-client-" + restclient.RequestTraceNameOpNamePlaceHolder,
		RetryCount:       0,
		RetryWaitTime:    0,
		RetryMaxWaitTime: 0,
		RetryOnHttpError: nil,
		Span:             nil,
	}

	urlBuilder := har.UrlBuilder{}
	urlBuilder.WithScheme(har.HttpScheme)
	urlBuilder.WithHostname(har.Localhost)
	urlBuilder.WithPort(ServerPort)
	urlBuilder.WithPath("/api/v1/example-post")

	reqBody := []byte("{ \"msg\": \"hello world\"}")
	/*
		request := restclient.Request{
			Method:      http.MethodPost,
			URL:         urlBuilder.Url(),
			HTTPVersion: "1.1",
			Cookies:     []restclient.Cookie{},
			Headers:     []restclient.NameValuePair{{Name: "Accept", Value: "application/json"}},
			QueryString: restclient.NameValuePairs{},
			PostData:    &restclient.PostData{MimeType: "application/json", Data: reqBody, Params: []restclient.Param{}},
			Comment:     "",
		}
	*/

	harTracingSpan := hartracing.GlobalTracer().StartSpan()
	client := restclient.NewClient(&cfg, restclient.WithHarSpan(harTracingSpan))
	defer client.Close()

	/*
		harTraceId := "63ef404fc936e44b4a000003:63ef404fc936e44b4a000003:63ef404fc936e44b4a000003"
		reqHeaders = append(reqHeaders, har.NameValuePair{Name: hartracing.HARTraceIdHeaderName, Value: harTraceId})
	*/
	reqHeaders := []har.NameValuePair{{Name: "Content-type", Value: "application/json"}, {Name: "Accept", Value: "application/json"}}
	request, err := client.NewRequest(http.MethodPost, urlBuilder.Url(), reqBody, reqHeaders, nil)
	require.NoError(t, err)

	harEntry, err := client.Execute("op2", "req-id", "", request, nil)
	var opts []har.BuilderOption
	opts = append(opts, har.WithEntry(harEntry))
	if err != nil {
		logHAR(t, har.NewHAR(opts...))
		require.NoError(t, err)
	}

	harEntry, err = client.Execute("op", "req-id", "lra-id", request, nil)
	opts = append(opts, har.WithEntry(harEntry))
	logHAR(t, har.NewHAR(opts...))
	require.NoError(t, err)

}

const (
	JAEGER_SERVICE_NAME = "JAEGER_SERVICE_NAME"
)

func logHAR(t *testing.T, har *har.HAR) {
	b, err := json.Marshal(har)
	require.NoError(t, err)
	t.Log(string(b))
}

func InitHarTracing(t *testing.T) (io.Closer, error) {
	trc, c := filetracer.NewTracer("/tmp")
	hartracing.SetGlobalTracer(trc)

	return c, nil
}

func InitTracing(t *testing.T) (io.Closer, error) {

	if os.Getenv(JAEGER_SERVICE_NAME) == "" {
		t.Log("skipping jaeger config no vars in env.... (" + JAEGER_SERVICE_NAME + ")")
		return nil, nil
	}

	var tracer opentracing.Tracer
	var closer io.Closer

	jcfg, err := jaegercfg.FromEnv()
	if err != nil {
		log.Warn().Err(err).Msg("Unable to configure JAEGER from environment")
		return nil, err
	}

	tracer, closer, err = jcfg.NewTracer(
		jaegercfg.Logger(&jlogger{}),
		jaegercfg.Metrics(metrics.NullFactory),
	)
	if nil != err {
		log.Error().Err(err).Msg("Error in NewTracer")
		return nil, err
	}

	opentracing.SetGlobalTracer(tracer)

	return closer, nil
}

type jlogger struct{}

func (l *jlogger) Error(msg string) {
	log.Error().Msg("(jaeger) " + msg)
}

func (l *jlogger) Infof(msg string, args ...interface{}) {
	log.Info().Msgf("(jaeger) "+msg, args...)
}
