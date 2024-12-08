//go:build tools

package bot

import (
	_ "entgo.io/ent/entc"
	_ "entgo.io/ent/entc/gen"
	_ "github.com/ogen-go/ent2ogen"
	_ "github.com/ogen-go/ogen"
	_ "github.com/ogen-go/ogen/conv"
	_ "github.com/ogen-go/ogen/gen"
	_ "github.com/ogen-go/ogen/gen/genfs"
	_ "github.com/ogen-go/ogen/middleware"
	_ "github.com/ogen-go/ogen/ogenerrors"
	_ "github.com/ogen-go/ogen/otelogen"
	_ "go.opentelemetry.io/otel/semconv/v1.19.0"
	_ "go.uber.org/mock/mockgen"
	_ "go.uber.org/mock/mockgen/model"
)
