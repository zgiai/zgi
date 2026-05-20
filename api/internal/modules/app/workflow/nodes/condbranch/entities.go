package condbranch

import "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"

type ComparisonOperator string

const (
	ComparisonOperatorContains    ComparisonOperator = "contains"
	ComparisonOperatorNotContains ComparisonOperator = "not contains"
	ComparisonOperatorStartWith   ComparisonOperator = "start with"
	ComparisonOperatorEndWith     ComparisonOperator = "end with"
	ComparisonOperatorIs          ComparisonOperator = "is"
	ComparisonOperatorIsNot       ComparisonOperator = "is not"
	ComparisonOperatorEmpty       ComparisonOperator = "empty"
	ComparisonOperatorNotEmpty    ComparisonOperator = "not empty"
	ComparisonOperatorIn          ComparisonOperator = "in"
	ComparisonOperatorNotIn       ComparisonOperator = "not in"
	ComparisonOperatorAllOf       ComparisonOperator = "all of"
	ComparisonOperatorEqual       ComparisonOperator = "="
	ComparisonOperatorNotEqual    ComparisonOperator = "≠"
	ComparisonOperatorGreaterThan ComparisonOperator = ">"
	ComparisonOperatorLessThan    ComparisonOperator = "<"
	ComparisonOperatorGreaterEq   ComparisonOperator = "≥"
	ComparisonOperatorLessEq      ComparisonOperator = "≤"
	ComparisonOperatorNull        ComparisonOperator = "null"
	ComparisonOperatorNotNull     ComparisonOperator = "not null"
	ComparisonOperatorExists      ComparisonOperator = "exists"
	ComparisonOperatorNotExists   ComparisonOperator = "not exists"
)

type LogicalOperator string

const (
	LogicalOperatorAnd LogicalOperator = "and"
	LogicalOperatorOr  LogicalOperator = "or"
)

type SubCondition struct {
	Key                string             `json:"key"`
	ComparisonOperator ComparisonOperator `json:"comparison_operator"`
	Value              any                `json:"value,omitempty"`
}

type SubVariableCondition struct {
	LogicalOperator LogicalOperator `json:"logical_operator"`
	Conditions      []SubCondition  `json:"conditions"`
}

type Condition struct {
	VariableSelector     []string              `json:"variable_selector"`
	ComparisonOperator   ComparisonOperator    `json:"comparison_operator"`
	Value                any                   `json:"value,omitempty"`
	SubVariableCondition *SubVariableCondition `json:"sub_variable_condition,omitempty"`
}

type Case struct {
	CaseID          string          `json:"case_id"`
	LogicalOperator LogicalOperator `json:"logical_operator"`
	Conditions      []Condition     `json:"conditions,omitempty"`
}

type NodeData struct {
	base.NodeData
	LogicalOperator LogicalOperator `json:"logical_operator,omitempty"`
	Conditions      []Condition     `json:"conditions,omitempty"`
	Cases           []Case          `json:"cases,omitempty"`
}

type Node struct {
	base.NodeStruct
	NodeData NodeData
}
