package v3

// Expand EXISTS filters, transforming them into semi-join apply expressions.
func maybeExpandExists(e *expr, filter, filterTop *expr) bool {
	if filter.op != existsOp {
		return false
	}

	t := *e
	t.removeFilter(filterTop)

	subquery := filter.inputs()[0]
	if subquery.op == projectOp {
		// Projections can be stripped from the right hand side of a
		// {anti,semi}-join because all we care about is existence.
		*subquery = *subquery.inputs()[0]
	}

	*e = expr{
		op: semiJoinOp,
		children: []*expr{
			&t,
			subquery,
		},
		props: t.props,
	}
	e.setApply()
	e.updateProps()
	return true
}

// Expand NOT EXISTS filters, transforming them into anti-join apply
// expressions.
func maybeExpandNotExists(e *expr, filter *expr) bool {
	if filter.op != notOp {
		return false
	}

	if !maybeExpandExists(e, filter.inputs()[0], filter) {
		return false
	}

	e.op = antiJoinOp
	return true
}

// Translate correlated join expressions into apply expressions.
func maybeExpandJoin(e *expr) {
	if e.op == innerJoinOp {
		left := e.inputs()[0]
		right := e.inputs()[1]
		if right.inputVars != 0 &&
			(right.inputVars&left.props.outputVars()) == right.inputVars {
			e.setApply()
		}
	}
}

// Expand correlated subqueries in filters into inner join apply expressions.
func maybeExpandFilter(e *expr, filter, filterTop *expr) bool {
	for _, input := range filter.inputs() {
		if input.isRelational() && input.inputVars != 0 &&
			(input.inputVars&e.props.outputVars()) == input.inputVars {
			// The input to the filter is relational and the relational expression
			// has free variables that are provided by the containing expression.

			// Make a copy of the subquery expression and replace the input to the
			// filter with a variable. Note that the subquery must have a single
			// output column in order to be usable in this context.
			subquery := *input
			*input = *subquery.props.columns[0].newVariableExpr("", subquery.props)
			updateProps(filterTop)

			// Replace "e" with an inner join apply expression where the left child
			// is the previous "e" and the right child is the subquery. The filter
			// expression which was previously on "e" is moved to the apply
			// expression.
			//
			// relational 1             inner join (apply)
			//   filter                   filter
			//     scalar          -->      scalar
			//       ...                      ...
			//       relational 2             variable
			//                            inputs
			//                              relational 1
			//                              relational 2

			t := *e
			t.removeFilter(filterTop)
			t.updateProps()

			*e = expr{
				op: innerJoinOp,
				children: []*expr{
					&t,
					&subquery,
				},
				props: t.props,
			}
			e.addFilter(filterTop)
			e.setApply()
			e.updateProps()
			return true
		}
	}

	for _, input := range filter.inputs() {
		if maybeExpandFilter(e, input, filterTop) {
			return true
		}
	}
	return false
}

// Recursively expand correlated subqueries into apply expressions.
func maybeExpandApply(e *expr) bool {
	for _, filter := range e.filters() {
		if maybeExpandExists(e, filter, filter) {
			return true
		}
		if maybeExpandNotExists(e, filter) {
			return true
		}
		if maybeExpandFilter(e, filter, filter) {
			return true
		}
	}

	maybeExpandJoin(e)
	return false
}

// apply(R, select(E)) -> select(apply(R, E))
func maybeDecorrelateSelection(e *expr) bool {
	right := e.inputs()[1]
	for _, filter := range right.filters() {
		if (filter.inputVars & e.props.outputVars()) != 0 {
			right.removeFilter(filter)
			right.updateProps()
			e.addFilter(filter)
			e.updateProps()
			return true
		}
	}
	return false
}

// apply(R, project(E)) -> project(apply(R, E))
func maybeDecorrelateProjection(e *expr) bool {
	// TODO(peter): unimplemented
	return false
}

// apply(R, join(A, B)) -> join(apply(R, A), apply(R, B))
func maybeDecorrelateJoin(e *expr) bool {
	// TODO(peter): unimplemented
	return false
}

// apply(R, groupBy(E)) -> groupBy(applyLOJ(R, E))
func maybeDecorrelateScalarGroupBy(e *expr) bool {
	// TODO(peter): unimplemented
	right := e.inputs()[1]
	if right.op == groupByOp && len(right.groupings()) == 0 {
		return true
	}
	return false
}

// apply(R, groupBy(E)) -> groupBy(apply(R, E))
func maybeDecorrelateVectorGroupBy(e *expr) bool {
	// TODO(peter): unimplemented
	return false
}

// Perform a single decorrelation transformation on an expression with the
// apply bit set. Returns true if a transformation was applied and returns
// false otherwise.
func maybeDecorrelate(e *expr) bool {
	if !e.hasApply() {
		return false
	}

	if maybeDecorrelateSelection(e) {
		return true
	}
	if maybeDecorrelateProjection(e) {
		return true
	}
	if maybeDecorrelateScalarGroupBy(e) {
		return false // TODO(peter): switch to true when implemented
	}

	e.clearApply()
	return false
}

// Recursively decorrelate the expression, expand correlated subqueries into
// apply expressions and then pushing down the apply expressions to leaves
// until they disappear or can no longer be pushed further.
func decorrelate(e *expr) {
	for maybeExpandApply(e) {
	}
	for maybeDecorrelate(e) {
	}

	for _, input := range e.inputs() {
		decorrelate(input)
	}
}
