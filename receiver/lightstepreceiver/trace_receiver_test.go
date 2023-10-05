// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lightstepreceiver

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestNew(t *testing.T) {
	type args struct {
		address      string
		nextConsumer consumer.Traces
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name:    "nil next Consumer",
			args:    args{},
			wantErr: component.ErrNilNextConsumer,
		},
		{
			name: "happy path",
			args: args{
				nextConsumer: consumertest.NewNop(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Protocols: Protocols{
					HTTP: &HTTPConfig{
						HTTPServerSettings: &confighttp.HTTPServerSettings{
							Endpoint: "0.0.0.0:443",
						},
					},
				},
			}

			got, err := newReceiver(cfg, tt.args.nextConsumer, receivertest.NewNopCreateSettings())
			require.Equal(t, tt.wantErr, err)
			if tt.wantErr == nil {
				require.NotNil(t, got)
			} else {
				require.Nil(t, got)
			}
		})
	}
}

