// Rectangularity is a precondition, not a type invariant: the request schema checks it at the edge.
export type MatrixRow = readonly number[];
export type Matrix = readonly MatrixRow[];
