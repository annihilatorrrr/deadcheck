// Licensed to Adam Shannon under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. The Moov Authors licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func Load(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("no path specified")
	}

	fullpath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("path %s expansion failed: %v", path, err)
	}

	var cfg Config

	fd, err := os.Open(fullpath)
	if err != nil {
		return nil, err
	}

	reader := viper.New()
	reader.SetConfigType("yaml")
	if err := reader.ReadConfig(fd); err != nil {
		return nil, err
	}
	if err := reader.UnmarshalExact(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

type Config struct {
	Checks []Check `yaml:"checks"`

	Alert Alert `yaml:"alert"`
}

type Check struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`

	Schedule ScheduleConfig `yaml:"schedule"`

	Alert Alert `yaml:"alert"`
}

type ScheduleConfig struct {
	Every       *time.Duration `yaml:"duration"`
	Weekdays    *PartialDay    `yaml:"weekdays"`
	BankingDays *PartialDay    `yaml:"bankingDays"`
}

type PartialDay struct {
	Timezone string  `yaml:"timezone"`
	Times    []Times `yaml:"times"`
}

type Times struct {
	At        string `yaml:"at"`
	Tolerance string `yaml:"tolerance"`
}

func (t Times) AtTime() (time.Time, error) {
	return time.Parse("15:04", t.At) // TODO(adam): parse more times
}

func (t Times) GetTolerance() (time.Duration, error) {
	return time.ParseDuration(t.Tolerance)
}

type Alert struct {
	PagerDuty *PagerDuty `yaml:"pagerduty"`
}

type PagerDuty struct {
	ApiKey           string `yaml:"apiKey"`
	EscalationPolicy string `yaml:"escalationPolicy"`

	// From is an email address of a valid user associated with the account making the request
	From string `yaml:"from"`

	RoutingKey string
}
