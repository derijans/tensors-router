package proxy

import "tensors-router/internal/schedulingcost"

type requestWorkHint struct {
	Work            schedulingcost.Work
	RequiredContext int
}
