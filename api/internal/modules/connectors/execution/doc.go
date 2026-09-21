// Package execution owns native Connector authorization and durable invocation
// claims. Router admits one immutable identity; Service validates ownership,
// target scope, both access constraints and the accepted operation before the
// native provider can submit. Terminal and unknown outcomes cannot replay.
package execution
