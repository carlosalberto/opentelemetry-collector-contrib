// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lightstepreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/lightstepreceiver"

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal/idutils"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/lightstepreceiver/internal/collectorpb"
)

const (
	// TODO: We need this constant?
	// Zipkin uses this as their http headers PLUS to be passed to obsreport
	receiverTransportV2PROTO  = "http_v2_proto"
)

// TODO: Confirm?
var errNextConsumerRespBody = []byte(`"Internal Server Error"`)

// lightstepReceiver type is used to handle spans received in the Lightstep format.
type lightstepReceiver struct {
	nextConsumer consumer.Traces

	shutdownWG sync.WaitGroup
	server     *http.Server
	config     *Config

	settings  receiver.CreateSettings
	//obsrecvrs map[string]*obsreport.Receiver // TODO - re-enable later?
}

var _ http.Handler = (*lightstepReceiver)(nil)

// newReceiver creates a new lightstepReceiver reference.
func newReceiver(config *Config, nextConsumer consumer.Traces, settings receiver.CreateSettings) (*lightstepReceiver, error) {
	if nextConsumer == nil {
		return nil, component.ErrNilNextConsumer
	}

	/*
	transports := []string{receiverTransportV1Thrift, receiverTransportV1JSON, receiverTransportV2JSON, receiverTransportV2PROTO}
	obsrecvrs := make(map[string]*obsreport.Receiver)
	for _, transport := range transports {
		obsrecv, err := obsreport.NewReceiver(obsreport.ReceiverSettings{
			ReceiverID:             settings.ID,
			Transport:              transport,
			ReceiverCreateSettings: settings,
		})
		if err != nil {
			return nil, err
		}
		obsrecvrs[transport] = obsrecv
	}*/

	lr := &lightstepReceiver{
		nextConsumer:             nextConsumer,
		config:                   config,
		settings:                 settings,
		//obsrecvrs:                obsrecvrs,
	}
	return lr, nil
}

// Start spins up the receiver's HTTP server and makes the receiver start its processing.
func (lr *lightstepReceiver) Start(_ context.Context, host component.Host) error {
	if host == nil {
		return errors.New("nil host")
	}

	var err error
	//lr.server, err = lr.config.HTTPServerSettings.ToServer(host, lr.settings.TelemetrySettings, lr)
	lr.server, err = lr.config.HTTP.ToServer(host, lr.settings.TelemetrySettings, lr)
	if err != nil {
		return err
	}

	var listener net.Listener
	//listener, err = lr.config.HTTPServerSettings.ToListener()
	listener, err = lr.config.HTTP.ToListener()
	if err != nil {
		return err
	}
	lr.shutdownWG.Add(1)
	go func() {
		defer lr.shutdownWG.Done()

		if errHTTP := lr.server.Serve(listener); !errors.Is(errHTTP, http.ErrServerClosed) && errHTTP != nil {
			host.ReportFatalError(errHTTP)
		}
	}()

	return nil
}

/*
// v2ToTraceSpans parses Lightstep v2 JSON or Protobuf traces and converts them to OpenCensus Proto spans.
func (lr *lightstepReceiver) v2ToTraceSpans(blob []byte, hdr http.Header) (reqs ptrace.Traces, err error) {
	// This flag's reference is from:
	//      https://github.com/openlightstep/lightstep-go/blob/3793c981d4f621c0e3eb1457acffa2c1cc591384/proto/v2/lightstep.proto#L154
	debugWasSet := hdr.Get("X-B3-Flags") == "1"

	// Lightstep can send protobuf via http
	if hdr.Get("Content-Type") == "application/x-protobuf" {
		// TODO: (@odeke-em) record the unique types of Content-Type uploads
		if debugWasSet {
		} else {
		}
	}
}*/

// Shutdown tells the receiver that should stop reception,
// giving it a chance to perform any necessary clean-up and shutting down
// its HTTP server.
func (lr *lightstepReceiver) Shutdown(context.Context) error {
	var err error
	if lr.server != nil {
		err = lr.server.Close()
	}
	lr.shutdownWG.Wait()
	return err
}

// processBodyIfNecessary checks the "Content-Encoding" HTTP header and if
// a compression such as "gzip", "deflate", "zlib", is found, the body will
// be uncompressed accordingly or return the body untouched if otherwise.
// Clients such as Lightstep-Java do this behavior e.g.
//
//	send "Content-Encoding":"gzip" of the JSON content.
func processBodyIfNecessary(req *http.Request) io.Reader {
	switch req.Header.Get("Content-Encoding") {
	default:
		return req.Body

	case "gzip":
		return gunzippedBodyIfPossible(req.Body)

	case "deflate", "zlib":
		return zlibUncompressedbody(req.Body)
	}
}

func gunzippedBodyIfPossible(r io.Reader) io.Reader {
	glr, err := gzip.NewReader(r)
	if err != nil {
		// Just return the old body as was
		return r
	}
	return glr
}

func zlibUncompressedbody(r io.Reader) io.Reader {
	lr, err := zlib.NewReader(r)
	if err != nil {
		// Just return the old body as was
		return r
	}
	return lr
}

// The lightstepReceiver receives spans from endpoint /api/v2 as JSON,
// unmarshalls them and sends them along to the nextConsumer.
func (lr *lightstepReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// TODO: How do we use path in our legacy tracers?
	// Now deserialize and process the spans.
	//asLightstepv1 := r.URL != nil && strings.Contains(r.URL.Path, "api/v1/spans")

	// TODO: do we check the header? proto?
	//obsrecv := lr.obsrecvrs[transportTag]
	//ctx = obsrecv.StartTracesOp(ctx)

	pr := processBodyIfNecessary(r)
	slurp, _ := io.ReadAll(pr)
	if c, ok := pr.(io.Closer); ok {
		_ = c.Close()
	}
	_ = r.Body.Close()

	// TODO: What to do if we get access token?
	// Probably we can drop them and simply have the Collector set them?
	var reportRequest = &collectorpb.ReportRequest{}
	err := proto.Unmarshal(slurp, reportRequest)

	// TODO: This how the microsats react usually?
	// Probably we send a different code? Which one we use for retries?
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var td ptrace.Traces
	td, err = TranslateToPData(reportRequest)
	if err != nil {
		// TODO: We should report marshaling didn't work out.
		// Unless we have proper controls to never fail, but
		// use safe values, etc
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	consumerErr := lr.nextConsumer.ConsumeTraces(ctx, td)

	/*
	receiverTagValue := lightstepV2TagValue
	if asLightstepv1 {
		receiverTagValue = lightstepV1TagValue
	}
	obsrecv.EndTracesOp(ctx, receiverTagValue, td.SpanCount(), consumerErr)*/

	// Likewise - what are the typical errors we sent back?
	if consumerErr != nil {
		// Transient error, due to some internal condition.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(errNextConsumerRespBody)
		return
	}

	// TODO: We do this, right?
	// Finally send back the response "Accepted" as
	// required at https://lightstep.io/lightstep-api/#/default/post_spans
	w.WriteHeader(http.StatusAccepted)
}

// TODO: Use me? Well, maybe not? grpc would use this channel?
/*
func transportType(r *http.Request) string {
	if r.Header.Get("Content-Type") == "application/x-protobuf" {
		return nil
	}
	// grpc?
	return nil
}*/
