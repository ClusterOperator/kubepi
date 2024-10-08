package system

import v1 "github.com/ClusterOperator/kubepi/internal/model/v1"

type OperationLog struct {
	v1.BaseModel        `storm:"inline"`
	v1.Metadata         `storm:"inline"`
	Operator            string `json:"operator"`
	Operation           string `json:"operation"`
	OperationDomain     string `json:"operationDomain"`
	SpecificInformation string `json:"specificInformation"`
}
