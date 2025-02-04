// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package humioexporter

import (
	"errors"
	"fmt"
	"net/url"
	"path"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	basePath         = "api/v1/ingest/"
	unstructuredPath = basePath + "humio-unstructured"
	structuredPath   = basePath + "humio-structured"
)

// LogsConfig represents the Humio configuration settings specific to logs
type LogsConfig struct {
	// The name of a custom log parser to use, if no parser is associated with the ingest token
	LogParser string `mapstructure:"log_parser"`
}

// TracesConfig represents the Humio configuration settings specific to traces
type TracesConfig struct {
	// Whether to use Unix timestamps, or to fall back to ISO 8601 formatted strings
	UnixTimestamps bool `mapstructure:"unix_timestamps"`
}

// Config represents the Humio configuration settings
type Config struct {
	// Inherited settings
	*config.ExporterSettings      `mapstructure:"-"`
	confighttp.HTTPClientSettings `mapstructure:",squash"`
	exporterhelper.QueueSettings  `mapstructure:"sending_queue"`
	exporterhelper.RetrySettings  `mapstructure:"retry_on_failure"`

	//Ingest token for identifying and authorizing with a Humio repository
	IngestToken string `mapstructure:"ingest_token"`

	// Endpoint for the unstructured ingest API, created internally
	unstructuredEndpoint *url.URL

	// Endpoint for the structured ingest API, created internally
	structuredEndpoint *url.URL

	// Whether gzip compression should be disabled when sending data to Humio
	DisableCompression bool `mapstructure:"disable_compression"`

	// Key-value pairs used to target specific data sources for storage inside Humio
	Tags map[string]string `mapstructure:"tags,omitempty"`

	// Whether this exporter should automatically add the service name as a tag
	DisableServiceTag bool `mapstructure:"disable_service_tag"`

	// Configuration options specific to logs
	Logs LogsConfig `mapstructure:"logs"`

	// Configuration options specific to traces
	Traces TracesConfig `mapstructure:"traces"`
}

// Validate ensures that a valid configuration has been provided, such that we can fail early
func (c *Config) Validate() error {
	if c.IngestToken == "" {
		return errors.New("requires an ingest_token")
	}

	if c.Endpoint == "" {
		return errors.New("requires an endpoint")
	}

	if c.DisableServiceTag && len(c.Tags) == 0 {
		return errors.New("requires at least one custom tag when disabling service tag")
	}

	// Ensure that it is possible to construct URLs to access the ingest API
	if _, err := c.getEndpoint(unstructuredPath); err != nil {
		return fmt.Errorf("unable to create URL for unstructured ingest API, endpoint %s is invalid", c.Endpoint)
	}

	// We require these headers, which should not be overwritten by the user
	if contentType, ok := c.Headers["content-type"]; ok && contentType != "application/json" {
		return errors.New("the Content-Type must be application/json, which is also the default for this header")
	}

	if _, ok := c.Headers["authorization"]; ok {
		return errors.New("the Authorization header must not be overwritten, since it is automatically generated from the ingest token")
	}

	if enc, ok := c.Headers["content-encoding"]; ok && (c.DisableCompression || enc != "gzip") {
		return errors.New("the Content-Encoding header must be gzip when using compression, and empty when compression is disabled")
	}

	return nil
}

// Sanitize ensures that the correct headers are inserted and that a url for each endpoint is obtainable
func (c *Config) sanitize() error {
	structured, errS := c.getEndpoint(structuredPath)
	unstructured, errU := c.getEndpoint(unstructuredPath)

	if errS != nil || errU != nil {
		return fmt.Errorf("badly formatted endpoint %s", c.Endpoint)
	}
	c.structuredEndpoint = structured
	c.unstructuredEndpoint = unstructured

	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}

	c.Headers["content-type"] = "application/json"
	c.Headers["authorization"] = "Bearer " + c.IngestToken

	if !c.DisableCompression {
		c.Headers["content-encoding"] = "gzip"
	}

	if _, ok := c.Headers["user-agent"]; !ok {
		c.Headers["user-agent"] = "opentelemetry-collector-contrib Humio"
	}

	return nil
}

// Get a URL for a specific destination path on the Humio endpoint
func (c *Config) getEndpoint(dest string) (*url.URL, error) {
	res, err := url.Parse(c.Endpoint)
	if err != nil {
		return res, err
	}

	res.Path = path.Join(res.Path, dest)
	return res, nil
}
