package filler_test

//
// import (
// 	"math/big"
// 	"math/rand"
// 	"testing/quick"
//
// 	"github.com/catalogfi/cobi/pkg/cobid/filler"
// 	"github.com/catalogfi/orderbook/model"
// 	"github.com/ethereum/go-ethereum/common/math"
//
// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// )
//
// var _ = Describe("Filler Strategy", func() {
// 	Context("Check if order matches the strategy", func() {
// 		Context("Checking order price", func() {
// 			It("should only accept order greater than or equal to the price", func() {
// 				stg := filler.Strategy{
// 					MinAmount: big.NewInt(1e8),
// 					MaxAmount: big.NewInt(1e9),
// 					Fee:       10,
// 				}
//
// 				test := func() bool {
// 					belowPrice := stg.Price() * rand.Float64()
// 					order1 := model.Order{
// 						FollowerAtomicSwap: &model.AtomicSwap{
// 							Amount: "200000000",
// 						},
// 						Price: belowPrice,
// 					}
// 					if stg.Match(order1) {
// 						return false
// 					}
//
// 					// skip when random float is 0
// 					factor := rand.Float64()
// 					if factor == 0 {
// 						return true
// 					}
// 					abovePrice := stg.Price() / factor
// 					order2 := model.Order{
// 						FollowerAtomicSwap: &model.AtomicSwap{
// 							Amount: "200000000",
// 						},
// 						Price: abovePrice,
// 					}
//
// 					return stg.Match(order2)
// 				}
//
// 				Expect(quick.Check(test, nil)).NotTo(HaveOccurred())
// 			})
// 		})
//
// 		Context("Checking order maker", func() {
// 			It("should accept any maker when maker list is nil", func() {
// 				stg := filler.Strategy{
// 					MinAmount: big.NewInt(1e8),
// 					MaxAmount: big.NewInt(1e9),
// 					Fee:       10,
// 				}
// 				test := func(maker string) bool {
// 					order := model.Order{
// 						Maker: maker,
// 						FollowerAtomicSwap: &model.AtomicSwap{
// 							Amount: "200000000",
// 						},
// 						Price: 1.5,
// 					}
// 					return stg.Match(order)
// 				}
//
// 				Expect(quick.Check(test, nil)).NotTo(HaveOccurred())
// 			})
//
// 			It("should only accept the order if its maker is in the list ", func() {
// 				whitelisted := "abc"
// 				stg := filler.Strategy{
// 					Makers:    []string{whitelisted},
// 					MinAmount: big.NewInt(1e8),
// 					MaxAmount: big.NewInt(1e9),
// 					Fee:       10,
// 				}
//
// 				test := func(maker string) bool {
// 					// Should be rejected
// 					order1 := model.Order{
// 						Maker: maker,
// 						FollowerAtomicSwap: &model.AtomicSwap{
// 							Amount: "200000000",
// 						},
// 						Price: 1.5,
// 					}
// 					if stg.Match(order1) {
// 						return false
// 					}
//
// 					// Should match
// 					order2 := model.Order{
// 						Maker: whitelisted,
// 						FollowerAtomicSwap: &model.AtomicSwap{
// 							Amount: "200000000",
// 						},
// 						Price: 1.5,
// 					}
// 					return stg.Match(order2)
// 				}
//
// 				Expect(quick.Check(test, nil)).NotTo(HaveOccurred())
// 			})
// 		})
//
// 		Context("Checking order amount", func() {
// 			It("should reject order smaller than the minimum amount", func() {
// 				stg := filler.Strategy{
// 					MinAmount: big.NewInt(1e8),
// 					MaxAmount: big.NewInt(1e9),
// 					Fee:       10,
// 				}
//
// 				test := func() bool {
// 					order1 := model.Order{
// 						FollowerAtomicSwap: &model.AtomicSwap{
// 							Amount: big.NewInt(rand.Int63n(stg.MinAmount.Int64())).String(),
// 						},
// 						Price: 1.5,
// 					}
// 					return !stg.Match(order1)
// 				}
//
// 				Expect(quick.Check(test, nil)).NotTo(HaveOccurred())
// 			})
//
// 			It("should reject order greater than the maximum amount", func() {
// 				stg := filler.Strategy{
// 					MinAmount: big.NewInt(1e8),
// 					MaxAmount: big.NewInt(1e9),
// 					Fee:       10,
// 				}
//
// 				test := func() bool {
// 					order1 := model.Order{
// 						FollowerAtomicSwap: &model.AtomicSwap{
// 							Amount: big.NewInt(rand.Int63n(math.MaxInt64-1e9) + 1e9).String(),
// 						},
// 						Price: 1.5,
// 					}
// 					return !stg.Match(order1)
// 				}
//
// 				Expect(quick.Check(test, nil)).NotTo(HaveOccurred())
// 			})
// 		})
// 	})
// })
