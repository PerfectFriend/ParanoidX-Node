// Package bridge provides WebSocket bridge to the simplex-chat CLI with auto-reconnect
package bridge

import (
	"fmt"
	"strings"

	"ParanoidX/internal/economy"
)

type CommandFunc func(args []string, chatID, dataDir string) string

func registerWalletCommands(dataDir string) map[string]CommandFunc {
	return map[string]CommandFunc{
		"/help":    cmdHelp,
		"/wallet":  cmdWallet,
		"/balance": cmdBalance,
		"/pay":     cmdPay,
		"/pos":     cmdPOS,
		"/mining":  cmdMining,
	}
}

func cmdHelp(args []string, chatID, dataDir string) string {
	return "🪙 **SimpleX Wallet Bot** 🪙\n\n" +
		"`/wallet <pubkey>` — check ng balance\n" +
		"`/balance <pubkey>` — same as /wallet\n" +
		"`/pay <from> <to> <amount>` — send ng\n" +
		"`/pos create <merchant> <amount> [desc]` — create POS invoice\n" +
		"`/pos pay <invoice_id> <payer>` — pay invoice\n" +
		"`/mining summary` — mining pool status\n" +
		"`/help` — this message\n\n" +
		"💎 Private. Self-custodial. Silver-backed."
}

func cmdWallet(args []string, chatID, dataDir string) string {
	if len(args) < 2 {
		return "Usage: `/wallet <pubkey>`"
	}
	pubkey := args[1]
	ledger := economy.LoadLedger(dataDir)
	bal := ledger.Balance(pubkey)
	return fmt.Sprintf("💰 **Balance**\n`%s`: `%d` ng = `%.4f` TLR", pubkey, bal, float64(bal)/float64(economy.NGPerTLR))
}

func cmdBalance(args []string, chatID, dataDir string) string {
	return cmdWallet(args, chatID, dataDir)
}

func cmdPay(args []string, chatID, dataDir string) string {
	if len(args) < 4 {
		return "Usage: `/pay <from_pubkey> <to_pubkey> <amount_ng>`"
	}
	from := args[1]
	to := args[2]
	amount := parseInt(args[3])
	if amount <= 0 {
		return "Amount must be > 0"
	}
	ledger := economy.LoadLedger(dataDir)
	if err := ledger.Transfer(from, to, amount); err != nil {
		return fmt.Sprintf("❌ Transfer failed: %v", err)
	}
	ledger.Save(dataDir)
	return fmt.Sprintf("✅ Transferred `%d` ng from `%s` to `%s`\nNew balance: `%d` ng", amount, from, to, ledger.Balance(from))
}

func cmdPOS(args []string, chatID, dataDir string) string {
	if len(args) < 2 {
		return "Usage:\n" +
			"`/pos create <merchant> <amount> [desc]`\n" +
			"`/pos pay <invoice_id> <payer>`"
	}
	sub := args[1]
	switch sub {
	case "create":
		if len(args) < 4 {
			return "Usage: `/pos create <merchant_pubkey> <amount_ng> [description]`"
		}
		merchant := args[2]
		amount := parseInt(args[3])
		desc := ""
		if len(args) > 4 {
			desc = strings.Join(args[4:], " ")
		}
		pm := economy.LoadPOSManager(dataDir)
		inv, err := pm.CreateInvoice(merchant, amount, desc)
		if err != nil {
			return fmt.Sprintf("❌ Failed: %v", err)
		}
		pm.Save(dataDir)
		return fmt.Sprintf("✅ Invoice created\nID: `%s`\nAmount: `%d` ng\nPayment URL: `%s`", inv.ID, inv.AmountNg, inv.PaymentURL)
	case "pay":
		if len(args) < 4 {
			return "Usage: `/pos pay <invoice_id> <payer_pubkey>`"
		}
		invoiceID := args[2]
		payer := args[3]
		ledger := economy.LoadLedger(dataDir)
		pm := economy.LoadPOSManager(dataDir)
		inv, err := pm.PayInvoice(invoiceID, payer, ledger)
		if err != nil {
			return fmt.Sprintf("❌ Payment failed: %v", err)
		}
		ledger.Save(dataDir)
		pm.Save(dataDir)
		return fmt.Sprintf("✅ Paid `%d` ng to `%s`\nNet: `%d` ng\nCommission: `%d` ng (1%%)",
			inv.AmountNg, inv.Merchant, inv.NetAmountNg, inv.CommissionNg)
	case "list":
		merchant := args[2]
		pm := economy.LoadPOSManager(dataDir)
		invoices := pm.ListMerchantInvoices(merchant)
		count := len(invoices)
		paid := 0
		for _, inv := range invoices {
			if inv.Status == "paid" {
				paid++
			}
		}
		rev := pm.MerchantRevenue(merchant)
		return fmt.Sprintf("📊 Merchant `%s`\nInvoices: %d (paid: %d)\nRevenue: %d ng", merchant, count, paid, rev)
	default:
		return "Unknown subcommand. Try `create`, `pay`, or `list`"
	}
}

func cmdMining(args []string, chatID, dataDir string) string {
	vm := economy.LoadVaultMining(dataDir)
	if len(args) > 1 && args[1] == "summary" {
		active := 0
		inactive := 0
		for _, p := range vm.Providers {
			if p.Active {
				active++
			} else {
				inactive++
			}
		}
		return fmt.Sprintf("⛏️ **Mining Pool**\nPool: `%d` ng deferred\nProviders: %d active, %d inactive\nNext payout: deferred 7-day schedule",
			vm.DeferredPoolNg, active, inactive)
	}
	return fmt.Sprintf("⛏️ Mining Pool: `%d` ng\nProviders: %d", vm.DeferredPoolNg, len(vm.Providers))
}

func parseInt(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}

func processWalletCommand(text string, chatID, dataDir string) string {
	text = strings.TrimSpace(text)
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return cmdHelp(nil, chatID, dataDir)
	}
	cmd := parts[0]
	cmds := registerWalletCommands(dataDir)
	if fn, ok := cmds[cmd]; ok {
		return fn(parts, chatID, dataDir)
	}
	return fmt.Sprintf("Unknown command `%s`. Try `/help`.", cmd)
}
