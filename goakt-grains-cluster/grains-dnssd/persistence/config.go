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

package persistence

import (
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/tochemey/gopack/postgres"
)

type Config struct {
	DBHost                string        `env:"DB_HOST"`                                   // DBHost represents the database host
	DBPort                int           `env:"DB_PORT"`                                   // DBPort is the database port
	DBName                string        `env:"DB_NAME"`                                   // DBName is the database name
	DBUser                string        `env:"DB_USER"`                                   // DBUser is the database user used to connect
	DBPassword            string        `env:"DB_PASSWORD"`                               // DBPassword is the database password
	DBSchema              string        `env:"DB_SCHEMA"`                                 // DBSchema represents the database schema
	MaxOpenConnections    int           `env:"MAX_OPEN_CONNECTIONS" envDefault:"25"`      // MaxOpenConnections represents the number of open connections in the pool
	MaxIdleConnections    int           `env:"MAX_IDLE_CONNECTIONS" envDefault:"25"`      // MaxIdleConnections represents the number of idle connections in the pool
	ConnectionMaxLifetime time.Duration `env:"CONNECTION_MAX_LIFETIME" envDefault:"5m0s"` // ConnectionMaxLifetime represents the connection max life time
}

// LoadConfig read the Postgres config from environment variables
func LoadConfig() *postgres.Config {
	config := &Config{}
	opts := env.Options{RequiredIfNoDef: true}
	if err := env.ParseWithOptions(config, opts); err != nil {
		// TODO: don't panic in production code
		panic(err)
	}

	return &postgres.Config{
		DBHost:                config.DBHost,
		DBPort:                config.DBPort,
		DBName:                config.DBName,
		DBUser:                config.DBUser,
		DBPassword:            config.DBPassword,
		DBSchema:              config.DBSchema,
		MaxConnections:        config.MaxOpenConnections,
		MinConnections:        0,
		MaxConnectionLifetime: config.ConnectionMaxLifetime,
		MaxConnIdleTime:       30 * time.Minute,
		HealthCheckPeriod:     time.Minute,
	}
}
