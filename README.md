# protoc-gen-go-nexus-temporal

A PoC Protobuf plugin for generating Temporal Nexus code.

## Installation

```
go install github.com/bergundy/protoc-gen-go-nexus-temporal/cmd/protoc-gen-go_nexus-temporal@latest
```

## Usage

### Create a proto file

`example.proto`

```protobuf
syntax="proto3";

package example.v1;

option go_package = "github.com/bergundy/greet-nexus-example/gen/example/v1;example";

message GreetInput {
  string name = 1;
}

message GreetOutput {
  string greeting = 1;
}

service Greeting {
  rpc Greet(GreetInput) returns (GreetOutput) {
  }
}
```

### Create `buf` config files

> NOTE: Alternatively you may use protoc directly.

`buf.yaml`

```yaml
version: v2
modules:
  - path: .
lint:
  use:
    - BASIC
  except:
    - FIELD_NOT_REQUIRED
    - PACKAGE_NO_IMPORT_CYCLE
breaking:
  use:
    - FILE
  except:
    - EXTENSION_NO_DELETE
    - FIELD_SAME_DEFAULT
```

`buf.gen.yaml`

```yaml
version: v2
managed:
  enabled: true
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/example/v1
    opt:
      - paths=source_relative
  - local: protoc-gen-go_nexus-temporal
    out: gen/example/v1
    strategy: all
    opt:
      - paths=source_relative
```

### Implement a service handler and register it with a Temporal worker

```go
package main

import (
	"context"

	example "github.com/bergundy/greet-nexus-example/gen/example/v1"
	"github.com/nexus-rpc/sdk-go/nexus"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporalnexus"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func GreetWorkflow(ctx workflow.Context, input *example.GreetInput) (*example.GreetOutput, error) {
	return &example.GreetOutput{Greeting: "Hello " + input.Name}, nil
}

type greetingHandler struct {
}

func (*greetingHandler) Greet(name string) nexus.Operation[*example.GreetInput, *example.GreetOutput] {
	return temporalnexus.NewWorkflowRunOperation(
		// The name of the Greet operation as defined in the proto.
		name,
        // Workflow to expose as the operation.
        // Input must match the operation input using this builder. See `NewWorkflowRunOperationWithOptions` for
        // exposing workflows with alternative signatures.
		GreetWorkflow,
		func(ctx context.Context, input *example.GreetInput, options nexus.StartOperationOptions) (client.StartWorkflowOptions, error) {
			return client.StartWorkflowOptions{
				ID: meaningfulBusinessID(input),
			}, nil
		})
}

func main() {
	c, _ := client.Dial(client.Options{HostPort: "localhost:7233"})
	w := worker.New(c, "example", worker.Options{})
    // All operations will automatically be registered on the service.
	example.RegisterGreetingNexusServiceHandler(w, &greetingHandler{})
    // Workflows need to be registered separately.
	w.RegisterWorkflow(GreetWorkflow)
}
```

### Invoke an operation from a workflow

#### Synchronous Call

```go
func CallerWorkflow(ctx workflow.Context) error {
	c := example.NewGreetingNexusClient("example-endpoint")
	output, err := c.ExecuteGreet(ctx, &example.GreetInput{Name: "World"}, workflow.NexusOperationOptions{})
	if err != nil {
		return err
	}
	workflow.GetLogger(ctx).Info("Got greeting", output.Greeting)
	return nil
}
```

#### Asynchronous Call

```
func CallerWorkflow(ctx workflow.Context) error {
	c := example.NewGreetingNexusClient("example-endpoint")
	fut := c.StartGreet(ctx, &example.GreetInput{Name: "World"}, workflow.NexusOperationOptions{})
	exec := workflow.NexusOperationExecution{}
	// Wait for operation to be started.
	if err := fut.GetNexusOperationExecution().Get(ctx, &exec); err != nil {
		return err
	}
	output, err := fut.GetTyped(ctx)
	if err != nil {
		return err
	}
	workflow.GetLogger(ctx).Info("Got greeting", output.Greeting)
	return nil
}
```
