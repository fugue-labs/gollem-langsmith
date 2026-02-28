package langsmith_test

import (
	"fmt"

	langsmith "github.com/fugue-labs/gollem-langsmith"
)

func ExampleNew() {
	h := langsmith.New(
		langsmith.WithProjectName("my-project"),
		langsmith.WithTags("production"),
	)
	defer h.Close()

	// Use h.Hook() with gollem agents:
	//   agent := core.NewAgent[string](model,
	//       core.WithHooks[string](h.Hook()),
	//   )

	fmt.Println("handler created")
	// Output: handler created
}
