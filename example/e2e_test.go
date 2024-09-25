package example

import (
	"context"
	"fmt"
	"testing"

	"github.com/bergundy/protoc-gen-go-nexus-temporal/gen/oms/v1"
	"github.com/nexus-rpc/sdk-go/nexus"
	"github.com/stretchr/testify/require"
	nexuspb "go.temporal.io/api/nexus/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporalnexus"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

type ordersHandler struct {
}

func CallerWorkflow(ctx workflow.Context) error {
	c := oms.NewOrdersNexusClient("orders")
	output, err := c.ExecuteCreateOrder(ctx, &oms.CreateOrderInput{CustomerId: "foo"}, workflow.NexusOperationOptions{})
	if err != nil {
		return err
	}
	fmt.Println(output.Order)
	return nil
}

func CreateOrderWorkflow(ctx workflow.Context, input *oms.CreateOrderInput) (*oms.CreateOrderOutput, error) {
	return &oms.CreateOrderOutput{Order: &oms.Order{Id: "abc"}}, nil
}

// CreateOrder implements oms.OrdersNexusServiceHandler.
func (*ordersHandler) CreateOrder(name string) nexus.Operation[*oms.CreateOrderInput, *oms.CreateOrderOutput] {
	return temporalnexus.NewWorkflowRunOperation(
		name,
		CreateOrderWorkflow,
		func(ctx context.Context, input *oms.CreateOrderInput, options nexus.StartOperationOptions) (client.StartWorkflowOptions, error) {
			return client.StartWorkflowOptions{
				ID: input.GetCustomerId(), // TODO
			}, nil
		})
}

func TestE2E(t *testing.T) {
	ctx := context.TODO()
	srv, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{
		ClientOptions: &client.Options{
			HostPort: "0.0.0.0:7233",
		},
		EnableUI: true,
		ExtraArgs: []string{
			"--http-port", "7243",
			"--dynamic-config-value", "system.enableNexus=true",
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Stop()) })

	c, err := client.Dial(client.Options{HostPort: "localhost:7233"})
	require.NoError(t, err)
	w := worker.New(c, "example", worker.Options{})
	oms.RegisterOrdersNexusServiceHandler(w, &ordersHandler{})
	w.RegisterWorkflow(CallerWorkflow)
	w.RegisterWorkflow(CreateOrderWorkflow)

	_, err = c.OperatorService().CreateNexusEndpoint(ctx, &operatorservice.CreateNexusEndpointRequest{
		Spec: &nexuspb.EndpointSpec{
			Name: "orders",
			Target: &nexuspb.EndpointTarget{
				Variant: &nexuspb.EndpointTarget_Worker_{
					Worker: &nexuspb.EndpointTarget_Worker{
						Namespace: "default",
						TaskQueue: "example",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, w.Start())
	t.Cleanup(w.Stop)

	fut, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		TaskQueue: "example",
	}, CallerWorkflow)
	require.NoError(t, err)
	require.NoError(t, fut.Get(ctx, nil))
}
