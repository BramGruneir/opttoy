package v3

import (
	"bytes"
)

func init() {
	registerOperator(unionOp, "union", union{})
	registerOperator(intersectOp, "intersect", nil)
	registerOperator(exceptOp, "except", nil)
}

type union struct{}

func (union) kind() operatorKind {
	return relationalKind
}

func (union) format(e *expr, buf *bytes.Buffer, level int) {
	formatRelational(e, buf, level)
	formatExprs(buf, "filters", e.filters(), level)
	formatExprs(buf, "inputs", e.inputs(), level)
}

func (union) updateProps(e *expr) {
	// Union is pass through and requires any input variables that its inputs
	// require.
	e.inputVars = 0
	for _, input := range e.inputs() {
		e.inputVars |= input.inputVars
		input.props.requiredOutputVars = input.props.outputVars()
	}

	// TODO(peter): update expr.props.
}
