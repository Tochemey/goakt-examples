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

// Config defines the service configuration
type Config struct {
	Port            int    `env:"PORT" envDefault:"50051"`
	ActorSystemName string `env:"SYSTEM_NAME" envDefault:"goakt-ai"`
	DiscoveryPort   int    `env:"DISCOVERY_PORT" envDefault:"3322"`
	PeersPort       int    `env:"PEERS_PORT" envDefault:"3320"`
	RemotingPort    int    `env:"REMOTING_PORT" envDefault:"50052"`
	DBHost          string `env:"DB_HOST" envDefault:"localhost"`
	DBPort          int    `env:"DB_PORT" envDefault:"5432"`
	DBName          string `env:"DB_NAME" envDefault:"goakt_ai"`
	DBUser          string `env:"DB_USER" envDefault:"goakt_ai"`
	DBPassword      string `env:"DB_PASSWORD" envDefault:""`
}

// GetConfig returns the configuration from environment
func GetConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
