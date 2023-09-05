package screener

import (
	"os"
	"strings"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func LocalPostgresDB() (*gorm.DB, error) {
	dns := os.Getenv("DB_DNS")
	db, err := gorm.Open(postgres.Open(dns), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := ClearDB(db); err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.Blacklist{}); err != nil {
		return nil, err
	}
	return db, nil
}

func ClearDB(db *gorm.DB) error {
	migrator := db.Migrator()
	tables, err := migrator.GetTables()
	if err != nil {
		return err
	}
	for _, table := range tables {
		if err := migrator.DropTable(table); err != nil {
			return err
		}
	}

	return nil
}

func addressMap(m map[string]model.Chain, addr string, chain model.Chain) map[string]model.Chain {
	if m == nil {
		m = map[string]model.Chain{}
	}
	m[addr] = chain
	return m
}

var _ = Describe("Screening blacklisted addresses", func() {
	Context("when sending a list of addresses to the API", func() {
		It("should return whether the addresses are blacklisted", func() {
			By("Set up the screener")
			db, err := LocalPostgresDB()
			Expect(err).Should(BeNil())
			screeningKey := os.Getenv("SCREENING_KEY")
			screener := NewScreener(db, screeningKey)

			By("Test a sanctioned address")
			addrs1 := addressMap(nil, "149w62rY42aZBox8fGcmqNsXUzSStKeq8C", model.Bitcoin)
			ok, err := screener.IsBlacklisted(addrs1)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).Should(BeTrue())

			By("Test a normal address")
			addrs2 := addressMap(nil, "0xEAF4a99DEA6fdc1e84996a2e61830222766D8303", model.Ethereum)
			ok, err = screener.IsBlacklisted(addrs2)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).Should(BeFalse())
		})
	})

	Context("when the address is in our blacklist", func() {
		It("should return the address been blacklisted", func() {
			By("Set up the screener")
			db, err := LocalPostgresDB()
			Expect(err).Should(BeNil())
			screeningKey := os.Getenv("SCREENING_KEY")
			screener := NewScreener(db, screeningKey)

			By("Update the table with a list of addresses")
			addrs := map[string]model.Chain{
				"123": model.Bitcoin,
				"abc": model.Bitcoin,
				"htp9mgp8tig923zfy7qf2zzbmumynefrahsp7vsg4wxv": model.Ethereum,
				"cezn7mqp9xoxn2hdyw6fjej73t7qax9rp2zys6hb3ieu": model.Ethereum,
				"5wwbygqg6bderm2nnnyumqxfcunb68b6kesxbywh1j3n": model.Ethereum,
				"geeccgj9bezvbvor1njkbcciqxjbxvedhaxdcrbdbmuy": model.Ethereum,
			}
			for addr := range addrs {
				blacklist := model.Blacklist{
					Address: addr,
				}
				Expect(db.Save(&blacklist).Error).Should(BeNil())
			}

			By("It should return the address is blacklisted")
			for addr := range addrs {
				input1 := addressMap(nil, addr, model.Ethereum)
				ok, err := screener.IsBlacklisted(input1)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).Should(BeTrue())

				input2 := addressMap(nil, addr, model.Bitcoin)
				ok, err = screener.IsBlacklisted(input2)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).Should(BeTrue())
			}

			By("It should query the external API if address not found in db")
			input := addressMap(nil, "149w62rY42aZBox8fGcmqNsXUzSStKeq8C", model.Bitcoin)
			ok, err := screener.IsBlacklisted(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).Should(BeTrue())

			By("It should be able to figure out different format of the same address")
			for addr := range addrs {

				// With up case letters
				input := addressMap(nil, strings.ToUpper(addr), model.Ethereum)
				ok, err := screener.IsBlacklisted(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).Should(BeTrue())

				// With whitespace
				input = addressMap(nil, addr+"  ", model.Ethereum)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).Should(BeTrue())
				input = addressMap(nil, "  "+addr, model.Ethereum)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).Should(BeTrue())

				// With 0x prefix
				input = addressMap(nil, "0x"+addr, model.Ethereum)
				ok, err = screener.IsBlacklisted(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).Should(BeTrue())
			}
		})
	})

	Context("when an address is blacklisted by the external api", func() {
		It("should store the result locally", func() {
			By("Set up the screener")
			db, err := LocalPostgresDB()
			Expect(err).Should(BeNil())
			screeningKey := os.Getenv("SCREENING_KEY")
			screener := NewScreener(db, screeningKey)

			By("Address should not be in the db")
			addr1 := "149w62rY42aZBox8fGcmqNsXUzSStKeq8C"
			formattedAddr := FormatAddress(addr1)
			var blacklist model.Blacklist
			err = db.Where("address = ?", formattedAddr).First(&blacklist).Error
			Expect(err).Should(Equal(gorm.ErrRecordNotFound))

			By("Test a sanctioned address")
			addrs := addressMap(nil, addr1, model.Bitcoin)
			ok, err := screener.IsBlacklisted(addrs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).Should(BeTrue())

			By("Address should be in the db")
			err = db.Where("address = ?", formattedAddr).First(&blacklist).Error
			Expect(err).Should(BeNil())
			Expect(blacklist.Address).Should(Equal(formattedAddr))
		})
	})

	Context("when not setting the trm key", func() {
		It("should not do any check", func() {
			By("Set up the screener")
			db, err := LocalPostgresDB()
			Expect(err).Should(BeNil())
			screener := NewScreener(db, "")

			By("Test a sanctioned address")
			addr1 := addressMap(nil, "149w62rY42aZBox8fGcmqNsXUzSStKeq8C", model.Bitcoin)
			ok, err := screener.IsBlacklisted(addr1)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).Should(BeFalse())
		})
	})
})
