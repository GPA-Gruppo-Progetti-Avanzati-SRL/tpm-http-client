package restclient_test

import (
	"GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-client/restclient"
	"encoding/json"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/har"
	"github.com/opentracing/opentracing-go"
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

func TestRestClient(t *testing.T) {

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
	urlBuilder.WithPort(3001)
	urlBuilder.WithPath("/api/v1/token-contexts/BPMIFI/tokens")

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

	client := restclient.NewClient(&cfg)
	defer client.Close()

	request, err := client.NewRequest(http.MethodPost, urlBuilder.Url(), reqBody, []har.NameValuePair{{Name: "Content-type", Value: "application/json"}, {Name: "Accept", Value: "application/json"}}, nil)
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
