// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package service

import "github.com/caarlos0/env/v11"

// Config defines the service configuration for DNSSD discovery
type Config struct {
	Port            int    `env:"PORT" envDefault:"50051"`
	DomainName      string `env:"DOMAIN_NAME"`
	ActorSystemName string `env:"SYSTEM_NAME" envDefault:"accounts"`
	TraceURL        string `env:"TRACE_URL" envDefault:"localhost:4317"`
	DiscoveryPort   int    `env:"DISCOVERY_PORT"`
	PeersPort       int    `env:"PEERS_PORT"`
	RemotingPort    int    `env:"REMOTING_PORT"`
	DBHost          string `env:"DB_HOST"`
	DBPort          int    `env:"DB_PORT"`
	DBName          string `env:"DB_NAME"`
	DBUser          string `env:"DB_USER"`
	DBPassword      string `env:"DB_PASSWORD"`
}

// GetConfig returns the configuration
func GetConfig() (*Config, error) {
	cfg := &Config{}
	opts := env.Options{RequiredIfNoDef: true, UseFieldNameByDefault: false}
	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, err
	}
	return cfg, nil
}
