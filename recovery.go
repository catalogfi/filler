package cobi

import (
	"time"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
)

func Recover(str store.UserStore, client rest.Client) error {
	for {
		orders, err := str.Orders()
		if err != nil {
			return err
		}
		for _, order := range orders {
			if order.Status >= store.InitiatorFailedToInitiate {
				eorder, err := client.GetOrder(order.ID)
				if err != nil {
					return err
				}
				if order.Status == store.InitiatorFailedToInitiate && eorder.InitiatorAtomicSwap.InitiateTxHash != "" {
					order.Status = store.InitiatorInitiated
				}
				if order.Status == store.FollowerFailedToInitiate && eorder.FollowerAtomicSwap.InitiateTxHash != "" {
					order.Status = store.FollowerInitiated
				}
				if order.Status == store.InitiatorFailedToRedeem && eorder.InitiatorAtomicSwap.RedeemTxHash != "" {
					order.Status = store.InitiatorRedeemed
				}
				if order.Status == store.FollowerFailedToRedeem && eorder.FollowerAtomicSwap.RedeemTxHash != "" {
					order.Status = store.FollowerRedeemed
				}
				if order.Status == store.InitiatorFailedToRefund && eorder.InitiatorAtomicSwap.RefundTxHash != "" {
					order.Status = store.InitiatorRefunded
				}
				if order.Status == store.FollowerFailedToRefund && eorder.FollowerAtomicSwap.RefundTxHash != "" {
					order.Status = store.FollowerRefunded
				}
				if order.Status < store.InitiatorFailedToInitiate {
					if err := str.PutStatus(order.SecretHash, order.Status); err != nil {
						return err
					}
				}
			}
		}
		time.Sleep(time.Minute)
	}
}
