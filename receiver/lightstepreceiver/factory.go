// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lightstepreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/lightstepreceiver"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/lightstepreceiver/internal/metadata"
)

// This file implements factory for Lightstep receiver.

const (
	// TODO: Define a new port for us to use.
	defaultBindEndpoint = "0.0.0.0:443"
)

// NewFactory creates a new Lightstep receiver factory
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		metadata.Type,
		createDefaultConfig,
		receiver.WithTraces(createTracesReceiver, metadata.TracesStability),
	)
}

// createDefaultConfig creates the default configuration for Lightstep receiver.
func createDefaultConfig() component.Config {
	return &Config{
		Protocols: Protocols {
			HTTP: &HTTPConfig {
				HTTPServerSettings: &confighttp.HTTPServerSettings{
					Endpoint: defaultBindEndpoint,
				},
			},
		},
	}
}

// createTracesReceiver creates a trace receiver based on provided config.
func createTracesReceiver(
	_ context.Context,
	set receiver.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (receiver.Traces, error) {
	rCfg := cfg.(*Config)
	return newReceiver(rCfg, nextConsumer, set)
}
