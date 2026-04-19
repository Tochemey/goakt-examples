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

package agents

import (
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// Names used when registering deterministic tools with the Tool sub-agent.
// Also used by the ToolExecutor router routee so the two tool dispatch paths
// resolve to the same handler.
const (
	ToolNameArithmetic = "arithmetic"
	ToolNamePercentOf  = "percent_of"
)

// Binary arithmetic operators accepted by the arithmetic tool and the
// ToolExecutor router. Kept as typed symbols so a typo in the switch
// statement surfaces at compile time instead of producing an
// "unsupported operator" error at runtime.
const (
	OpAdd      = "+"
	OpSubtract = "-"
	OpMultiply = "*"
	OpDivide   = "/"
)

// ArithmeticArgs mirrors the regex-based arithmetic the legacy ToolAgent
// supported: a numeric pair with one of +, -, *, /. The tool returns an error
// for unsupported operators or division by zero so the LLM sees the failure
// and can decide how to recover.
type ArithmeticArgs struct {
	A  float64 `json:"a"`
	Op string  `json:"op"`
	B  float64 `json:"b"`
}

// ArithmeticResult carries the formatted numeric result, matching the legacy
// "%.2f" formatting of ToolAgent.tryMath so downstream observers see the same
// output shape.
type ArithmeticResult struct {
	Result string `json:"result"`
}

// PercentArgs mirrors the "X% of Y" pattern the legacy ToolAgent matched.
type PercentArgs struct {
	Percent float64 `json:"percent"`
	Value   float64 `json:"value"`
}

// PercentResult is the percent-of-value output.
type PercentResult struct {
	Result string `json:"result"`
}

// newCalculatorTool wraps the arithmetic fallback as an ADK tool.
func newCalculatorTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        ToolNameArithmetic,
		Description: "Computes a simple two-operand arithmetic expression. Supported ops: +, -, *, /.",
	}, func(_ tool.Context, args ArithmeticArgs) (ArithmeticResult, error) {
		var result float64

		switch args.Op {
		case OpAdd:
			result = args.A + args.B
		case OpSubtract:
			result = args.A - args.B
		case OpMultiply:
			result = args.A * args.B
		case OpDivide:
			if args.B == 0 {
				return ArithmeticResult{}, fmt.Errorf("division by zero")
			}
			result = args.A / args.B
		default:
			return ArithmeticResult{}, fmt.Errorf("unsupported operator: %q", args.Op)
		}

		return ArithmeticResult{Result: fmt.Sprintf("%.2f", result)}, nil
	})
}

// newPercentTool wraps the percent-of helper as an ADK tool.
func newPercentTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        ToolNamePercentOf,
		Description: "Computes Percent% of Value and returns the numeric answer.",
	}, func(_ tool.Context, args PercentArgs) (PercentResult, error) {
		return PercentResult{Result: fmt.Sprintf("%.2f", args.Value*(args.Percent/100))}, nil
	})
}

// builtinTools returns the tools wired into the Tool sub-agent. Construction
// may fail at startup if the ADK schema inference chokes on a new Go version;
// the caller should surface that error during actor system bootstrap.
func builtinTools() ([]tool.Tool, error) {
	calculatorTool, err := newCalculatorTool()
	if err != nil {
		return nil, fmt.Errorf("calculator tool: %w", err)
	}

	percentTool, err := newPercentTool()
	if err != nil {
		return nil, fmt.Errorf("percent tool: %w", err)
	}

	return []tool.Tool{calculatorTool, percentTool}, nil
}

// RunArithmetic executes a deterministic arithmetic operation. Shared with
// the ToolExecutor router routee so both the ADK path and the legacy
// ExecuteTool path produce identical output for identical inputs.
func RunArithmetic(leftOperand float64, operator string, rightOperand float64) (string, error) {
	switch operator {
	case OpAdd:
		return fmt.Sprintf("%.2f", leftOperand+rightOperand), nil
	case OpSubtract:
		return fmt.Sprintf("%.2f", leftOperand-rightOperand), nil
	case OpMultiply:
		return fmt.Sprintf("%.2f", leftOperand*rightOperand), nil
	case OpDivide:
		if rightOperand == 0 {
			return "", fmt.Errorf("division by zero")
		}
		return fmt.Sprintf("%.2f", leftOperand/rightOperand), nil
	default:
		return "", fmt.Errorf("unsupported operator: %q", operator)
	}
}

// RunPercent executes a percent-of calculation. Shared with the ToolExecutor
// router routee.
func RunPercent(percent, value float64) (string, error) {
	return fmt.Sprintf("%.2f", value*(percent/100)), nil
}
