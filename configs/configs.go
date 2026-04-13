// Package configs embeds the default YAML configuration files for the services.
package configs

import _ "embed"

//go:embed producer.yaml
var ProducerYAML []byte

//go:embed consumer.yaml
var ConsumerYAML []byte
