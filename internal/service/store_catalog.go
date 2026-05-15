package service

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/shopspring/decimal"
)

type StoreCatalogItem struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Prices      map[string]string `json:"prices"`
	Content     string            `json:"content,omitempty"`
}

type StoreSessionIntent string

const (
	StoreIntentUserDeposit  StoreSessionIntent = "user_deposit"
	StoreIntentPurchase     StoreSessionIntent = "purchase"
	StoreIntentUserWithdraw StoreSessionIntent = "user_withdraw"
)

type StoreSessionData struct {
	Intent    StoreSessionIntent `json:"intent"`
	Amount    string             `json:"amount,omitempty"`
	ItemID    json.RawMessage    `json:"item_id,omitempty"`
	ItemPrice string             `json:"item_price,omitempty"`
}

func sortedAssets(homeBlockchains map[string]uint64) []string {
	out := make([]string, 0, len(homeBlockchains))
	for asset := range homeBlockchains {
		out = append(out, strings.ToLower(strings.TrimSpace(asset)))
	}
	sort.Strings(out)
	return out
}

func supportedStoreAssets(homeBlockchains map[string]uint64) []string {
	configured := make(map[string]struct{}, len(homeBlockchains))
	for _, asset := range sortedAssets(homeBlockchains) {
		configured[asset] = struct{}{}
	}

	out := make([]string, 0, 2)
	for _, asset := range []string{"yusd", "yellow"} {
		if _, ok := configured[asset]; ok {
			out = append(out, asset)
		}
	}
	if len(out) == 0 {
		out = append(out, "yusd")
	}
	return out
}

func normalizeHomeBlockchains(homeBlockchains map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(homeBlockchains))
	for asset, chainID := range homeBlockchains {
		normalized := strings.ToLower(strings.TrimSpace(asset))
		if normalized == "" {
			continue
		}
		out[normalized] = chainID
	}
	return out
}

func defaultChannelBootstrapAmounts() map[string]string {
	return map[string]string{
		"yusd":   "10",
		"yellow": "10",
	}
}

func normalizeChannelBootstrapAmounts(raw map[string]string) map[string]string {
	out := defaultChannelBootstrapAmounts()
	for asset, amount := range raw {
		normalized := strings.ToLower(strings.TrimSpace(asset))
		amount = strings.TrimSpace(amount)
		if normalized == "" || amount == "" {
			continue
		}
		out[normalized] = amount
	}
	return out
}

func (s *WalletStoreService) channelBootstrapAmount(asset string) string {
	if amount := strings.TrimSpace(s.channelBootstrapAmounts[strings.ToLower(strings.TrimSpace(asset))]); amount != "" {
		return amount
	}
	return "10"
}

func strictBalancesForAsset(allocations []app.AppAllocationV1, userAddress string, appAddress string, asset string) (decimal.Decimal, decimal.Decimal, error) {
	if len(allocations) != 2 {
		return decimal.Zero, decimal.Zero, conflictf("app allocations must contain exactly wallet and app signer")
	}

	userAmount := decimal.Zero
	appAmount := decimal.Zero
	seenUser := false
	seenApp := false
	for _, allocation := range allocations {
		if !strings.EqualFold(allocation.Asset, asset) {
			return decimal.Zero, decimal.Zero, conflictf("app allocation asset does not match selected asset")
		}
		if allocation.Amount.IsNegative() {
			return decimal.Zero, decimal.Zero, conflictf("app allocation amount cannot be negative")
		}
		switch {
		case strings.EqualFold(allocation.Participant, userAddress):
			if seenUser {
				return decimal.Zero, decimal.Zero, conflictf("duplicate wallet allocation")
			}
			seenUser = true
			userAmount = allocation.Amount
		case strings.EqualFold(allocation.Participant, appAddress):
			if seenApp {
				return decimal.Zero, decimal.Zero, conflictf("duplicate app signer allocation")
			}
			seenApp = true
			appAmount = allocation.Amount
		default:
			return decimal.Zero, decimal.Zero, conflictf("unexpected app allocation participant")
		}
	}
	if !seenUser || !seenApp {
		return decimal.Zero, decimal.Zero, conflictf("app allocations must include wallet and app signer")
	}
	return userAmount, appAmount, nil
}

func displayBalancesForAsset(allocations []app.AppAllocationV1, userAddress string, appAddress string, asset string) (decimal.Decimal, decimal.Decimal) {
	userAmount := decimal.Zero
	appAmount := decimal.Zero
	for _, allocation := range allocations {
		if !strings.EqualFold(allocation.Asset, asset) || allocation.Amount.IsNegative() {
			continue
		}
		switch {
		case strings.EqualFold(allocation.Participant, userAddress):
			userAmount = userAmount.Add(allocation.Amount)
		case strings.EqualFold(allocation.Participant, appAddress):
			appAmount = appAmount.Add(allocation.Amount)
		}
	}
	return userAmount, appAmount
}

func currentBalancesForAsset(allocations []app.AppAllocationV1, userAddress string, appAddress string, asset string) (decimal.Decimal, decimal.Decimal, error) {
	userAmount := decimal.Zero
	appAmount := decimal.Zero
	seenUser := false
	seenApp := false
	for _, allocation := range allocations {
		if !strings.EqualFold(allocation.Asset, asset) {
			continue
		}
		if allocation.Amount.IsNegative() {
			return decimal.Zero, decimal.Zero, conflictf("app allocation amount cannot be negative")
		}
		switch {
		case strings.EqualFold(allocation.Participant, userAddress):
			if seenUser {
				return decimal.Zero, decimal.Zero, conflictf("duplicate wallet allocation")
			}
			seenUser = true
			userAmount = allocation.Amount
		case strings.EqualFold(allocation.Participant, appAddress):
			if seenApp {
				return decimal.Zero, decimal.Zero, conflictf("duplicate app signer allocation")
			}
			seenApp = true
			appAmount = allocation.Amount
		default:
			return decimal.Zero, decimal.Zero, conflictf("unexpected app allocation participant")
		}
	}
	return userAmount, appAmount, nil
}

func openClosedStatus(isClosed bool) string {
	if isClosed {
		return "closed"
	}
	return "open"
}

func sessionDataItemID(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", invalidf("purchase session_data.item_id is required")
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return "", invalidf("purchase session_data.item_id is required")
		}
		return asString, nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return asNumber.String(), nil
	}

	return "", invalidf("purchase session_data.item_id must be a string or number")
}

func seededCatalog() []StoreCatalogItem {
	return []StoreCatalogItem{
		{
			ID:          "1",
			Title:       "Designing Instant Micropayments",
			Description: "Short-form reading on instant purchases and content gating with app sessions.",
			Type:        "article",
			Prices: map[string]string{
				"yellow": "1.35",
				"yusd":   "0.9",
			},
			Content: "Designing Instant Micropayments\n\nMicropayment UX succeeds when top-level product actions stay simple and settlement details remain observable but hidden.",
		},
		{
			ID:          "2",
			Title:       "State Channel Monthly",
			Description: "A lightweight issue about channels, signatures, and settlement UX.",
			Type:        "magazine",
			Prices: map[string]string{
				"yellow": "2.00",
				"yusd":   "1.00",
			},
			Content: "State Channel Monthly\n\nFeature stories on settlement rails, app sessions, and why product UX should hide protocol noise.",
		},
		{
			ID:          "3",
			Title:       "Nitronode Field Guide",
			Description: "A practical guide to following app-session state from wallet signature to Nitronode submission.",
			Type:        "guide",
			Prices: map[string]string{
				"yellow": "1.80",
				"yusd":   "1.25",
			},
			Content: "Nitronode Field Guide\n\nA good store flow lets operators trace app-session versions, signatures, and allocation deltas without exposing that complexity to shoppers.",
		},
		{
			ID:          "4",
			Title:       "Wallet UX Patterns",
			Description: "Small interface patterns for keeping wallet prompts understandable and recoverable.",
			Type:        "playbook",
			Prices: map[string]string{
				"yellow": "1.10",
				"yusd":   "0.75",
			},
			Content: "Wallet UX Patterns\n\nWallet UX works best when each signature has a clear product reason, visible state changes, and a straightforward recovery path.",
		},
		{
			ID:          "5",
			Title:       "Channel Recovery Checklist",
			Description: "A concise checklist for reconciling local state with the latest channel state.",
			Type:        "checklist",
			Prices: map[string]string{
				"yellow": "2.25",
				"yusd":   "1.50",
			},
			Content: "Channel Recovery Checklist\n\nConfirm the app session, compare the expected version, verify session data, reconcile pending purchases, then expose owned content only after ownership is submitted.",
		},
	}
}
