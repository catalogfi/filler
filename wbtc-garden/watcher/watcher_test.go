package watcher_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/mocks"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"gorm.io/gorm"

	. "github.com/catalogfi/cobi/wbtc-garden/watcher"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

var _ = Describe("Watcher", func() {
	defer GinkgoRecover()

	logger := zap.NewNop()
	var (
		mockCtrl    *gomock.Controller
		mockWatcher *mocks.MockWatcher
		mockStore   *mocks.MockStore

		mockError  = errors.New("mock error")
		mockTxHash = "mock tx hash"
		mockAmount = "mock amount"
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockWatcher = mocks.NewMockWatcher(mockCtrl)
		mockStore = mocks.NewMockStore(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("update swap status when status is NotStarted", func() {
		It("should return an error when IsDetected fails", func() {
			mockWatcher.EXPECT().IsDetected().Return(false, "", "", mockError)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.NotStarted}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(swap.Status).Should(Equal(model.NotStarted))
			Expect(err).To(HaveOccurred())
		})

		It("should update initiate tx hash and amount when they are different from values in swap", func() {
			mockWatcher.EXPECT().IsDetected().Return(false, mockTxHash, mockAmount, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.NotStarted}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.Status).Should(Equal(model.NotStarted))
			Expect(swap.InitiateTxHash).Should(Equal(mockTxHash))
			Expect(swap.FilledAmount).Should(Equal(mockAmount))
			Expect(err).To(BeNil())
		})

		It("should not update initiate tx hash and amount when they are same as the values in swap", func() {
			mockWatcher.EXPECT().IsDetected().Return(false, mockTxHash, mockAmount, nil)
			_, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.NotStarted, InitiateTxHash: mockTxHash, FilledAmount: mockAmount}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(err).To(BeNil())
		})

		It("should update amount, initiateTxHash and status when isDetected returns true for fully filled", func() {
			mockWatcher.EXPECT().IsDetected().Return(true, mockTxHash, mockAmount, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.NotStarted}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.Status).Should(Equal(model.Detected))
			Expect(swap.InitiateTxHash).Should(Equal(mockTxHash))
			Expect(swap.FilledAmount).Should(Equal(mockAmount))
			Expect(err).To(BeNil())
		})
	})

	Describe("update swap status when status is Detected", func() {
		It("should return an error when Status fails", func() {
			mockWatcher.EXPECT().Status(mockTxHash).Return(uint64(0), uint64(0), false, mockError)
			_, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Detected, InitiateTxHash: mockTxHash}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(err).To(HaveOccurred())
		})

		It("should update confirmations and block number when status returns valid values", func() {
			mockWatcher.EXPECT().Status(mockTxHash).Return(uint64(10), uint64(1), false, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Detected, InitiateTxHash: mockTxHash, CurrentConfirmations: 0, MinimumConfirmations: 2}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.CurrentConfirmations).Should(Equal(uint64(1)))
			Expect(swap.InitiateBlockNumber).Should(Equal(uint64(10)))
			Expect(swap.Status).Should(Equal(model.Detected))
			Expect(err).To(BeNil())
		})

		It("should update confirmations to min confirmations even if the confirmations are higher", func() {
			mockWatcher.EXPECT().Status(mockTxHash).Return(uint64(10), uint64(3), false, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Detected, InitiateTxHash: mockTxHash, CurrentConfirmations: 1, MinimumConfirmations: 2}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.CurrentConfirmations).Should(Equal(uint64(2)))
			Expect(swap.InitiateBlockNumber).Should(Equal(uint64(10)))
			Expect(swap.Status).Should(Equal(model.Initiated))
			Expect(err).To(BeNil())
		})

		It("should update status to Initiated when confirmations are equal to min confirmations", func() {
			mockWatcher.EXPECT().Status(mockTxHash).Return(uint64(10), uint64(2), false, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Detected, InitiateTxHash: mockTxHash, CurrentConfirmations: 1, MinimumConfirmations: 2}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.CurrentConfirmations).Should(Equal(uint64(2)))
			Expect(swap.InitiateBlockNumber).Should(Equal(uint64(10)))
			Expect(swap.Status).Should(Equal(model.Initiated))
			Expect(err).To(BeNil())
		})

		It("should not update status to Initiated when confirmations are less than min confirmations", func() {
			mockWatcher.EXPECT().Status(mockTxHash).Return(uint64(10), uint64(1), false, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Detected, InitiateTxHash: mockTxHash, CurrentConfirmations: 1, MinimumConfirmations: 2}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(swap.Status).Should(Equal(model.Detected))
			Expect(err).To(BeNil())
		})
	})

	Describe("update swap status when status is Initiated", func() {
		It("should return an error when IsRedeemed fails", func() {
			mockWatcher.EXPECT().IsRedeemed().Return(false, nil, "", mockError)
			_, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Initiated}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(err).To(HaveOccurred())
		})

		It("should update status to Redeemed when IsRedeemed returns true", func() {
			secret := [32]byte{}
			rand.Read(secret[:])
			mockWatcher.EXPECT().IsRedeemed().Return(true, secret[:], mockTxHash, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Initiated}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.Status).Should(Equal(model.Redeemed))
			Expect(swap.RedeemTxHash).Should(Equal(mockTxHash))
			Expect(swap.Secret).Should(Equal(hex.EncodeToString(secret[:])))
			Expect(err).To(BeNil())
		})

		It("should not update status to Redeemed when IsRedeemed returns false", func() {
			mockWatcher.EXPECT().IsRedeemed().Return(false, nil, "", nil).AnyTimes()
			mockWatcher.EXPECT().Expired().Return(false, mockError).AnyTimes()
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Initiated}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(swap.Status).Should(Equal(model.Initiated))
			Expect(err).To(HaveOccurred())
		})

		It("should update status to expired when expired is true", func() {
			mockWatcher.EXPECT().IsRedeemed().Return(false, nil, "", nil).AnyTimes()
			mockWatcher.EXPECT().Expired().Return(true, nil).AnyTimes()
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Initiated}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.Status).Should(Equal(model.Expired))
			Expect(err).To(BeNil())
		})

		It("should not update status to Redeemed when IsRedeemed returns false", func() {
			mockWatcher.EXPECT().IsRedeemed().Return(false, nil, "", nil).AnyTimes()
			mockWatcher.EXPECT().Expired().Return(false, nil).AnyTimes()
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Initiated}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(swap.Status).Should(Equal(model.Initiated))
			Expect(err).To(BeNil())
		})
	})

	Describe("update swap status when status is Expired", func() {
		It("should return an error when IsRefunded fails", func() {
			mockWatcher.EXPECT().IsRefunded().Return(false, "", mockError)
			_, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Expired}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(err).To(HaveOccurred())
		})

		It("should update status to Refunded when IsRefunded returns true", func() {
			mockWatcher.EXPECT().IsRefunded().Return(true, mockTxHash, nil)
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Expired}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.Status).Should(Equal(model.Refunded))
			Expect(swap.RefundTxHash).Should(Equal(mockTxHash))
			Expect(err).To(BeNil())
		})

		It("should not update status to Redeemed when IsRedeemed returns false", func() {
			mockWatcher.EXPECT().IsRefunded().Return(false, "", nil).AnyTimes()
			mockWatcher.EXPECT().IsRedeemed().Return(false, nil, "", mockError).AnyTimes()
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Expired}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(swap.Status).Should(Equal(model.Expired))
			Expect(err).To(HaveOccurred())
		})

		It("should update status to expired when expired is true", func() {
			secret := [32]byte{}
			rand.Read(secret[:])
			mockWatcher.EXPECT().IsRefunded().Return(false, "", nil).AnyTimes()
			mockWatcher.EXPECT().IsRedeemed().Return(true, secret[:], mockTxHash, nil).AnyTimes()
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Expired}, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(swap.Status).Should(Equal(model.Redeemed))
			Expect(swap.RedeemTxHash).Should(Equal(mockTxHash))
			Expect(swap.Secret).Should(Equal(hex.EncodeToString(secret[:])))
			Expect(err).To(BeNil())
		})

		It("should not update status to Redeemed when IsRedeemed returns false", func() {
			mockWatcher.EXPECT().IsRefunded().Return(false, "", nil).AnyTimes()
			mockWatcher.EXPECT().IsRedeemed().Return(false, nil, "", nil).AnyTimes()
			swap, cond, err := UpdateSwapStatus(logger, model.AtomicSwap{Status: model.Expired}, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(swap.Status).Should(Equal(model.Expired))
			Expect(err).To(BeNil())
		})
	})

	Describe("update order status when status is Filled", func() {
		It("should return an error if initiator or follower atomic swap fails", func() {
			mockWatcher.EXPECT().IsDetected().Return(false, "", "", mockError)
			order, cond, err := UpdateStatus(logger, model.Order{Status: model.Filled, InitiatorAtomicSwap: &model.AtomicSwap{Status: model.Redeemed}, FollowerAtomicSwap: &model.AtomicSwap{Status: model.NotStarted}}, mockWatcher, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(order.Status).Should(Equal(model.Filled))
			Expect(err).To(HaveOccurred())
		})

		It("should not update status when there is no state change in atomic swaps", func() {
			order, cond, err := UpdateStatus(logger, model.Order{Status: model.Filled, InitiatorAtomicSwap: &model.AtomicSwap{Status: model.Redeemed}, FollowerAtomicSwap: &model.AtomicSwap{Status: model.Redeemed}}, mockWatcher, mockWatcher)
			Expect(cond).To(BeFalse())
			Expect(order.Status).Should(Equal(model.Filled))
			Expect(err).To(BeNil())
		})

		It("should update order status to executed when both swaps are redeemed", func() {
			secret := [32]byte{}
			rand.Read(secret[:])
			mockWatcher.EXPECT().IsRedeemed().Return(true, secret[:], mockTxHash, nil)
			order, cond, err := UpdateStatus(logger, model.Order{Status: model.Filled, InitiatorAtomicSwap: &model.AtomicSwap{Status: model.Redeemed}, FollowerAtomicSwap: &model.AtomicSwap{Status: model.Initiated}}, mockWatcher, mockWatcher)
			Expect(cond).To(BeTrue())
			Expect(order.Status).Should(Equal(model.Executed))
			Expect(err).To(BeNil())
		})
	})

	Describe("should be able to process an order", func() {
		It("should hard fail when initiator redeems and refunds", func() {
			order := model.Order{
				Status: model.Filled,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
			}
			updatedOrder := model.Order{
				Status: model.FailedHard,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeTrue())
			Expect(updatedOrder.Status).To(Equal(model.FailedHard))
			Expect(err).To(BeNil())
		})

		It("should hard fail when follower redeems and refunds", func() {
			order := model.Order{
				Status: model.Filled,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
			}
			updatedOrder := model.Order{
				Status: model.FailedHard,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeTrue())
			Expect(updatedOrder.Status).To(Equal(model.FailedHard))
			Expect(err).To(BeNil())
		})

		It("should soft fail when both initiator and follower refunds", func() {
			order := model.Order{
				Status: model.Filled,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
			}
			updatedOrder := model.Order{
				Status: model.FailedSoft,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeTrue())
			Expect(updatedOrder.Status).To(Equal(model.FailedSoft))
			Expect(err).To(BeNil())
		})

		It("should soft fail when both initiator refunds and follower does not start", func() {
			order := model.Order{
				Status: model.Filled,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder := model.Order{
				Status: model.FailedSoft,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeTrue())
			Expect(updatedOrder.Status).To(Equal(model.FailedSoft))
			Expect(err).To(BeNil())
		})
		It("should cancel an order if OrderTimeout passes", func() {
			createdAt := time.Now().Add(-OrderTimeout - time.Minute)
			order := model.Order{
				Status: model.Created,
				Model: gorm.Model{
					CreatedAt: createdAt,
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder := model.Order{
				Status: model.Cancelled,
				Model: gorm.Model{
					CreatedAt: createdAt,
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeTrue())
			Expect(updatedOrder.Status).To(Equal(model.Cancelled))
			Expect(err).To(BeNil())
		})

		It("should not panic if the order status fails", func() {
			createdAt := time.Now().Add(-OrderTimeout - time.Minute)
			order := model.Order{
				Status: model.Created,
				Model: gorm.Model{
					CreatedAt: createdAt,
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder := model.Order{
				Status: model.Cancelled,
				Model: gorm.Model{
					CreatedAt: createdAt,
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeTrue())
			Expect(updatedOrder.Status).To(Equal(model.Cancelled))
			Expect(err).To(BeNil())
		})

		It("should cancel an order if OrderTimeout passes", func() {
			createdAt := time.Now().Add(-SwapInitiationTimeout - time.Minute)
			order := model.Order{
				Status: model.Filled,
				Model: gorm.Model{
					CreatedAt: createdAt,
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder := model.Order{
				Status: model.Cancelled,
				Model: gorm.Model{
					CreatedAt: createdAt,
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeTrue())
			Expect(updatedOrder.Status).To(Equal(model.Cancelled))
			Expect(err).To(BeNil())
		})
		It("should not update order if the status is created and order does not expire", func() {
			order := model.Order{
				Status: model.Created,
				Model: gorm.Model{
					CreatedAt: time.Now(),
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeFalse())
			Expect(updatedOrder.Status).To(Equal(model.Created))
			Expect(err).To(BeNil())
		})

		It("should fail to load initiator watcher if config and status are not provided", func() {
			order := model.Order{
				Status: model.Filled,
				Model: gorm.Model{
					CreatedAt: time.Now(),
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			updatedOrder, ctn, err := ProcessOrder(order, mockStore, model.Network{}, logger)
			Expect(ctn).To(BeFalse())
			Expect(updatedOrder.Status).To(Equal(model.Filled))
			Expect(err).To(HaveOccurred())
		})

		It("should fail to load follower watcher if config and status are not provided", func() {
			order := model.Order{
				Status: model.Filled,
				Model: gorm.Model{
					CreatedAt: time.Now(),
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status:           model.NotStarted,
					InitiatorAddress: "n2psi3r4BpvzjPPXdaz3de1k1MgNi4Wyzd",
					RedeemerAddress:  "n1g3aBR4dhZnhwzT4PhfoaVB2doYJ5JvpX",
					Amount:           "100000",
					Timelock:         "100",
					Chain:            model.BitcoinTestnet,
					Asset:            model.Primary,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}
			ProcessOrder(order, mockStore, model.Network{
				model.BitcoinTestnet: model.NetworkConfig{
					RPC: map[string]string{"mempool": "https://mempool.space/testnet/api"},
				},
			}, logger)
		})

		It("should fail to load follower watcher if config and status are not provided", func() {
			order := model.Order{
				Status: model.Filled,
				Model: gorm.Model{
					CreatedAt: time.Now(),
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status:           model.NotStarted,
					InitiatorAddress: "n2psi3r4BpvzjPPXdaz3de1k1MgNi4Wyzd",
					RedeemerAddress:  "n1g3aBR4dhZnhwzT4PhfoaVB2doYJ5JvpX",
					Amount:           "100000",
					Timelock:         "100",
					Chain:            model.BitcoinTestnet,
					Asset:            model.Primary,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status:           model.NotStarted,
					InitiatorAddress: "n2psi3r4BpvzjPPXdaz3de1k1MgNi4Wyzd",
					RedeemerAddress:  "n1g3aBR4dhZnhwzT4PhfoaVB2doYJ5JvpX",
					Amount:           "100000",
					Timelock:         "100",
					Chain:            model.BitcoinTestnet,
					Asset:            model.Primary,
				},
			}
			ProcessOrder(order, mockStore, model.Network{
				model.BitcoinTestnet: model.NetworkConfig{
					RPC: map[string]string{"mempool": "https://mempool.space/testnet/api"},
				},
			}, logger)
		})
	})

	Describe("should be able to run the watcher", func() {
		defer GinkgoRecover()

		var minWorkers = 4

		It("should build a watcher", func() {
			watcher := NewWatcher(logger, mockStore, model.Network{}, 1)
			Expect(watcher).ToNot(BeNil())
			order := model.Order{
				Status: model.Filled,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
			}
			updatedOrder := model.Order{
				Status: model.FailedHard,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
			}

			mockStore.EXPECT().GetActiveOrders().Return([]model.Order{order}, nil).MaxTimes(2)
			mockStore.EXPECT().UpdateOrder(&updatedOrder).Return(nil)

			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()
			watcher.Run(ctx)
		})

		It("should build a watcher", func() {
			watcher := NewWatcher(logger, mockStore, model.Network{}, minWorkers)
			Expect(watcher).ToNot(BeNil())
			order := model.Order{
				Status: model.Filled,
				Model: gorm.Model{
					CreatedAt: time.Now(),
				},
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.NotStarted,
				},
			}

			mockStore.EXPECT().GetActiveOrders().Return([]model.Order{order}, nil).MaxTimes(2)
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()
			watcher.Run(ctx)
		})

		It("should build a watcher", func() {
			watcher := NewWatcher(logger, mockStore, model.Network{}, minWorkers)
			Expect(watcher).ToNot(BeNil())
			order := model.Order{
				Status: model.Filled,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
			}
			updatedOrder := model.Order{
				Status: model.FailedHard,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
			}

			mockStore.EXPECT().GetActiveOrders().Return([]model.Order{order}, nil).AnyTimes()
			mockStore.EXPECT().UpdateOrder(&updatedOrder).Return(mockError)
			ctx, _ := context.WithTimeout(context.Background(), 7*time.Second)
			watcher.Run(ctx)
		})

		It("should build a watcher", func() {
			watcher := NewWatcher(logger, mockStore, model.Network{}, minWorkers)
			Expect(watcher).ToNot(BeNil())
			mockStore.EXPECT().GetActiveOrders().Return(nil, mockError).AnyTimes()
			ctx, _ := context.WithTimeout(context.Background(), 8*time.Second)
			watcher.Run(ctx)
		})

		It("should build a watcher", func() {
			order := model.Order{
				Status: model.Filled,
				InitiatorAtomicSwap: &model.AtomicSwap{
					Status: model.Refunded,
				},
				FollowerAtomicSwap: &model.AtomicSwap{
					Status: model.Redeemed,
				},
			}
			watcher := NewWatcher(logger, mockStore, model.Network{}, 0)
			Expect(watcher).ToNot(BeNil())
			mockStore.EXPECT().GetActiveOrders().Return([]model.Order{order}, nil)
			ctx, _ := context.WithTimeout(context.Background(), 8*time.Second)
			watcher.Run(ctx)
		})
	})
})
