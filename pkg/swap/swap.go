package swap

type Action string

var (
	ActionInitiate Action = "initiate"
	ActionRedeem   Action = "redeem"
	ActionRefund   Action = "refund"
)
