// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lightstepreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/lightstepreceiver"

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal/idutils"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/lightstepreceiver/internal/collectorpb"
)

// TODO: Can we return an error here? That's what other receivers do?
func TranslateToPData(req *collectorpb.ReportRequest) (ptrace.Traces, error) {
	// I think GetSpans() creates a new thing?
	td := ptrace.NewTraces()
	if len(req.GetSpans()) == 0 {
		return td, nil
	}

	// QUESTION: Can slices ([]) be nil?
	// Need to check this is null?
	reporter := req.GetReporter()
	//reporterId := reporter.GetReporterId() // this one - how is it used by the backend?

	// TODO: Can our legacy tracers include no component name?
	rss := td.ResourceSpans().AppendEmpty()
	resource := rss.Resource()
	translateTagsToAttrs(reporter.GetTags(), resource.Attributes())
	serviceName := getServiceName(reporter.GetTags())
	resource.Attributes().PutStr("service.name", serviceName)

	//rs.SetSchemaUrl() // what? to use?
	sss := rss.ScopeSpans().AppendEmpty()
	scope := sss.Scope()
	scope.SetName("lightstep-exporter") // TODO: What does Zipkin/Jaeger do here?
	scope.SetVersion("0.0.1")
	spans := sss.Spans()
	spans.EnsureCapacity(len(req.GetSpans())) // This doesn't create a copy, no?

	for _, lspan := range req.GetSpans() {
		span := spans.AppendEmpty()
		span.SetName(lspan.GetOperationName())
		translateTagsToAttrs(lspan.GetTags(), span.Attributes())

		ts := lspan.GetStartTimestamp()
		startt := time.Unix(ts.GetSeconds(), int64(ts.GetNanos()))

		duration, err := time.ParseDuration(fmt.Sprintf("%dus", lspan.GetDurationMicros()))
		if err != nil {
			// TODO: Consider logging this one.
			continue
		}
		// TODO: Double check this one? See the actual typical "topology"
		endt := startt.Add(duration)
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(startt))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(endt))

		// TODO: Verify this is the correct order.
		span.SetTraceID(idutils.UInt64ToTraceID(0, lspan.GetSpanContext().GetTraceId()))
		span.SetSpanID(idutils.UInt64ToSpanID(lspan.GetSpanContext().GetSpanId()))
		setSpanParents(span, lspan.GetReferences())

		translateLogsToEvents(span, lspan.GetLogs())

		// span.Status().SetStatusCode()
		// TODO: For error, we need to check GetTags(), each, and map the "error" with true to
		// SpanStatus.ERROR
	}

	// TODO: Check that the simple Unmarshal() call is fine? Ask Alex.

	return td, nil
}

// The first check takes for granted attrs is empty. Remove it?
// TODO: consider namespacing our own proto, e.g. "lightstep_proto" as internal.collectorpb
func translateTagsToAttrs(tags []*collectorpb.KeyValue, attrs pcommon.Map) {
	attrs.EnsureCapacity(len(tags)) // Needed at all?
	for _, kv := range tags {
		key := kv.GetKey()
		value := kv.GetValue()
		switch x := value.(type) {
		case *collectorpb.KeyValue_StringValue:
			attrs.PutStr(key, x.StringValue)
		case *collectorpb.KeyValue_JsonValue:
			attrs.PutStr(key, x.JsonValue)
		case *collectorpb.KeyValue_IntValue:
			attrs.PutInt(key, x.IntValue)
		case *collectorpb.KeyValue_DoubleValue:
			attrs.PutDouble(key, x.DoubleValue)
		case *collectorpb.KeyValue_BoolValue:
			attrs.PutBool(key, x.BoolValue)
		}
	}
}

func getServiceName(tags []*collectorpb.KeyValue) string {
	for _, tag := range tags {
		if tag.GetKey() == "lightstep.component_name" {
			return tag.GetStringValue()
		}
	}

	return ""
}

func setSpanParents(span ptrace.Span, refs []*collectorpb.Reference) {
	links := span.Links()
	is_main_parent_set := false
	for _, ref := range refs {
		if !is_main_parent_set {
			span.SetParentSpanID(idutils.UInt64ToSpanID(ref.GetSpanContext().GetSpanId()))
			is_main_parent_set = true
		} else {
			// TODO: Optimize, i.e. we only get Links if references is larger than 1
			link := links.AppendEmpty()
			link.SetSpanID(idutils.UInt64ToSpanID(ref.GetSpanContext().GetSpanId()));
			link.SetTraceID(idutils.UInt64ToTraceID(0, ref.GetSpanContext().GetTraceId()))
		}
	}
}

func translateLogsToEvents(span ptrace.Span, logs []*collectorpb.Log) {
	if len(logs) == 0 {
		return
	}

	events := span.Events()
	for _, log := range logs {
		tstamp := time.Unix(log.GetTimestamp().GetSeconds(), int64(log.GetTimestamp().GetNanos()))
		event := events.AppendEmpty()
		event.SetTimestamp(pcommon.NewTimestampFromTime(tstamp))
		translateTagsToAttrs(log.GetFields(), event.Attributes())
		// What about name? Check the OT mapping
	}
}
