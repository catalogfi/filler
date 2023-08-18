package cobi

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func Retry(entropy []byte, store Store) *cobra.Command {
	var (
		orderId uint
	)

	var cmd = &cobra.Command{
		Use:   "retry",
		Short: "Retry an order",
		Run: func(c *cobra.Command, args []string) {
			order, err := store.GetOrder(orderId)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while fetching order : %v", err))
				return
			}
			// so the idea behind retry in bussiness logic is to change status in store object
			// by changing status in store object watcher will try to re-execute the order
			// in order to reset appropriate status we are subtracting 6 from current status
			// statuses are in a sequence resulting in subtraction of 6 leading to its appropriate previous status
			if order.Status >= FollowerRefunded {
				store.PutStatus(order.SecretHash, order.Status-6)
			}
		},
		DisableAutoGenTag: true,
	}

	cmd.Flags().UintVar(&orderId, "order-id", 0, "order id")
	cmd.MarkFlagRequired("order-id")
	return cmd
}

func Update() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "update",
		Short: "Update COBI to the latest version",
		Run: func(c *cobra.Command, args []string) {
			if err := exec.Command("curl", "https://cobi-releases.s3.ap-south-1.amazonaws.com/update.sh", "-sSfL | sh").Run(); err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to update cobi : %v", err))
				return
			}
		},
		DisableAutoGenTag: true,
	}
	return cmd
}
