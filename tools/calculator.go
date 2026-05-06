package tools

// calculator.go demonstrates the OPTIONAL PARAMETERS pattern for ADK FunctionTools.
//
// ADK Go rules for required vs optional fields:
//
//   Required  — field has NO omitempty in the json tag.
//               The LLM MUST supply this argument; ADK returns an error if it doesn't.
//
//   Optional  — field has `json:"name,omitempty"` (or omitzero).
//               The LLM MAY omit it; the zero value of the type is used when absent.
//
// Contrast with tools.go (weather/time) where every field is required.

import (
	"fmt"
	"math"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// calcArgs mixes required and optional parameters.
type calcArgs struct {
	// Operation is required — the LLM must always provide it.
	Operation string `json:"operation" jsonschema:"The arithmetic operation to perform: add, subtract, multiply, divide, power, sqrt."`

	// A is required — first operand.
	A float64 `json:"a" jsonschema:"First operand (required for all operations except sqrt)."`

	// B is optional — omitempty means ADK marks it as not required in the JSON schema.
	// The LLM may omit B when calling sqrt (which only needs A).
	B float64 `json:"b,omitempty" jsonschema:"Second operand (required for add/subtract/multiply/divide/power; omit for sqrt)."`

	// Precision is optional with a default of 2 decimal places.
	Precision int `json:"precision,omitempty" jsonschema:"Number of decimal places in the result (default: 2, max: 10)."`
}

// calcResult is the structured return type.
type calcResult struct {
	Expression string  `json:"expression"`
	Result     float64 `json:"result"`
	Formatted  string  `json:"formatted"`
}

// calculate performs the requested arithmetic operation.
func calculate(_ tool.Context, args calcArgs) (calcResult, error) {
	prec := args.Precision
	if prec <= 0 {
		prec = 2
	}
	if prec > 10 {
		prec = 10
	}

	var result float64
	var expr string

	switch args.Operation {
	case "add":
		result = args.A + args.B
		expr = fmt.Sprintf("%g + %g", args.A, args.B)
	case "subtract":
		result = args.A - args.B
		expr = fmt.Sprintf("%g - %g", args.A, args.B)
	case "multiply":
		result = args.A * args.B
		expr = fmt.Sprintf("%g × %g", args.A, args.B)
	case "divide":
		if args.B == 0 {
			return calcResult{}, fmt.Errorf("division by zero")
		}
		result = args.A / args.B
		expr = fmt.Sprintf("%g ÷ %g", args.A, args.B)
	case "power":
		result = math.Pow(args.A, args.B)
		expr = fmt.Sprintf("%g ^ %g", args.A, args.B)
	case "sqrt":
		if args.A < 0 {
			return calcResult{}, fmt.Errorf("cannot take square root of a negative number")
		}
		result = math.Sqrt(args.A)
		expr = fmt.Sprintf("√%g", args.A)
	default:
		return calcResult{}, fmt.Errorf("unknown operation %q: must be add, subtract, multiply, divide, power, or sqrt", args.Operation)
	}

	fmtStr := fmt.Sprintf("%%.%df", prec)
	return calcResult{
		Expression: expr,
		Result:     result,
		Formatted:  fmt.Sprintf(fmtStr, result),
	}, nil
}

// NewCalculatorTool creates the calculate FunctionTool.
// The args struct mixes required (operation, a) and optional (b, precision) fields,
// showing how the LLM receives a schema where some arguments are optional.
func NewCalculatorTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "calculate",
		Description: "Performs arithmetic operations: add, subtract, multiply, divide, power, sqrt. B is optional for sqrt. Precision is optional (default 2).",
	}, calculate)
	if err != nil {
		panic(fmt.Sprintf("NewCalculatorTool: %v", err))
	}
	return t
}
