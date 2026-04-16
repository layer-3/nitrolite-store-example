package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/layer-3/nitrolite-go-example/internal/config"
)

func openAPIHandler(cfg *config.Config) http.HandlerFunc {
	spec := buildOpenAPISpec(cfg)
	payload, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}
}

func buildOpenAPISpec(cfg *config.Config) map[string]any {
	exampleAsset, exampleChain := openAPIAssetAndChain(cfg.HomeBlockchains)
	if exampleAsset == "" {
		exampleAsset = "yusd"
	}
	if exampleChain == 0 {
		exampleChain = 11155111
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Nitrolite Go Example API",
			"version":     "1.0.0",
			"description": "App Session Micropayment Store reference API for the Nitrolite Go example. The public product surface is a content store at `/`, while `/reference` and `/advanced` remain hidden developer surfaces for direct route exploration.",
		},
		"servers": []map[string]any{{"url": "/"}},
		"tags": []map[string]any{
			{"name": "Health / Wallet / Node"},
			{"name": "Store"},
			{"name": "Catalog"},
			{"name": "Content"},
			{"name": "Raw Channel"},
			{"name": "Raw Sessions"},
			{"name": "Raw Session Keys"},
			{"name": "Auth"},
			{"name": "Legacy"},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "API key",
				},
				"writeSessionCookie": map[string]any{
					"type": "apiKey",
					"in":   "cookie",
					"name": writeSessionCookieName,
				},
			},
			"schemas": map[string]any{
				"ErrorEnvelope": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"code":    map[string]any{"type": "string"},
								"message": map[string]any{"type": "string"},
							},
							"required": []string{"code", "message"},
						},
					},
					"required": []string{"error"},
				},
				"UnlockRequest": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"api_key": map[string]any{"type": "string"},
					},
					"required": []string{"api_key"},
				},
				"UnlockStatus": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"unlocked":   map[string]any{"type": "boolean"},
						"expires_at": map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"GenericObject": genericObjectSchema(),
			},
		},
		"paths": map[string]any{
			"/healthz": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Process health", "Current process and clearnode connectivity.", "", map[string]any{"status": "ok", "clearnode": "connected", "signer": "0xabc"}),
			},
			"/readyz": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Readiness", "Readiness after SDK initialization and ping.", "", map[string]any{"status": "ready"}),
			},
			"/api/v1/auth/status": map[string]any{
				"get": readOperation("Auth", "Write unlock status", "Returns whether writes are unlocked for the current browser session.", "UnlockStatus", map[string]any{"unlocked": false}),
			},
			"/api/v1/auth/unlock": map[string]any{
				"post": writeAccessOperation("Auth", "Unlock writes", "Verifies the demo write key and sets a short-lived HttpOnly write-session cookie.", requestBody("UnlockRequest", map[string]any{"api_key": "paste-demo-key-here"}), "UnlockStatus", map[string]any{"unlocked": true}),
			},
			"/api/v1/auth/lock": map[string]any{
				"post": writeAccessOperation("Auth", "Lock writes", "Clears the browser write-session cookie.", nil, "UnlockStatus", map[string]any{"unlocked": false}),
			},
			"/api/v1/wallet": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Wallet", "Current backend signer address and configured home chains.", "", map[string]any{
					"address":         "0x562679665e9fb0ed7a4251effe9672553e9a5406",
					"homeBlockchains": map[string]any{exampleAsset: exampleChain},
				}),
			},
			"/api/v1/node/config": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Node config", "Supported chains and node metadata.", "", map[string]any{
					"nodeAddress": "0xc76632d91d45ec88304ab2a983451d9edf908c0d",
					"blockchains": []map[string]any{{"id": exampleChain, "name": "ethereum_sepolia"}},
				}),
			},
			"/api/v1/node/blockchains": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Supported blockchains", "Lists chains exposed by the node.", "", map[string]any{
					"blockchains": []map[string]any{{"id": exampleChain, "name": "ethereum_sepolia"}},
				}),
			},
			"/api/v1/node/assets": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Supported assets", "Lists supported assets and token addresses.", "", map[string]any{
					"assets": []map[string]any{{
						"name":                  "Yellow USD",
						"symbol":                exampleAsset,
						"decimals":              6,
						"suggestedBlockchainID": exampleChain,
					}},
				}),
			},
			"/api/v1/store/config": map[string]any{
				"get": readOperation("Store", "Store config", "Returns store metadata, supported assets, and the seeded catalog summary. The first request also ensures a browser-scoped store session cookie exists.", "", map[string]any{
					"browser_session_id": "browser_123",
					"store": map[string]any{
						"store_name":       cfg.StoreName,
						"app_id":           cfg.StoreAppID,
						"default_asset":    exampleAsset,
						"supported_assets": []string{exampleAsset},
						"catalog":          []map[string]any{{"id": "book-go-101", "title": "Go Systems Handbook"}},
					},
				}),
			},
			"/api/v1/catalog": map[string]any{
				"get": readOperation("Catalog", "Catalog listing", "Lists the store catalog filtered to a selected asset.", "", map[string]any{
					"items": []map[string]any{{
						"id":          "book-go-101",
						"title":       "Go Systems Handbook",
						"type":        "book",
						"description": "A practical field guide to Go services.",
						"prices":      map[string]any{exampleAsset: "2.50"},
					}},
				}),
			},
			"/api/v1/catalog/{id}": map[string]any{
				"get": readOperation("Catalog", "Catalog item", "Returns one catalog item for the requested asset.", "", map[string]any{
					"item": map[string]any{
						"id":          "book-go-101",
						"title":       "Go Systems Handbook",
						"type":        "book",
						"description": "A practical field guide to Go services.",
						"prices":      map[string]any{exampleAsset: "2.50"},
					},
				}),
			},
			"/api/v1/store/session": map[string]any{
				"get": readOperation("Store", "Current store session", "Returns the browser-scoped store session summary for one asset. If no session exists yet, the status is `missing`.", "", map[string]any{
					"session": map[string]any{
						"asset":             exampleAsset,
						"status":            "missing",
						"available_balance": "5",
						"user_allocation":   "0",
						"app_allocation":    "0",
					},
				}),
			},
			"/api/v1/store/session/create": map[string]any{
				"post": map[string]any{
					"tags":        []string{"Store"},
					"summary":     "Create or load store session",
					"description": "Creates one browser-scoped app session for the selected asset if it does not already exist, or returns the existing one.",
					"requestBody": requestBody("", map[string]any{"asset": exampleAsset}),
					"responses": standardResponses(http.StatusOK, "", map[string]any{
						"session": map[string]any{
							"asset":           exampleAsset,
							"app_session_id":  "0xsession",
							"status":          "open",
							"version":         1,
							"user_allocation": "0",
							"app_allocation":  "0",
						},
					}, true),
				},
			},
			"/api/v1/app-session/submit-state": map[string]any{
				"post": map[string]any{
					"tags":        []string{"Store"},
					"summary":     "Submit store app-session state",
					"description": "Single store mutation endpoint. The server inspects `session_data.action` and dispatches deposit, purchase, `user_withdraw`, or hidden `app_withdraw` logic. Purchase pricing is always revalidated against the server catalog.",
					"requestBody": requestBody("", map[string]any{
						"session_id":   "0xsession",
						"asset":        exampleAsset,
						"session_data": "{\"action\":\"purchase\",\"item_id\":\"book-go-101\",\"price\":\"2.50\"}",
					}),
					"responses": standardResponses(http.StatusOK, "", map[string]any{
						"session": map[string]any{
							"asset":           exampleAsset,
							"app_session_id":  "0xsession",
							"status":          "open",
							"version":         2,
							"user_allocation": "2.50",
							"app_allocation":  "2.50",
							"session_data":    "{\"action\":\"purchase\",\"item_id\":\"book-go-101\",\"price\":\"2.50\"}",
						},
					}, true),
				},
			},
			"/api/v1/purchases": map[string]any{
				"get": readOperation("Content", "Purchased items", "Lists purchased items for the current browser-scoped store identity and selected asset.", "", map[string]any{
					"purchases": []map[string]any{{
						"item_id":        "book-go-101",
						"asset":          exampleAsset,
						"app_session_id": "0xsession",
						"version":        2,
					}},
				}),
			},
			"/api/v1/content/{id}": map[string]any{
				"get": readOperation("Content", "Purchased content", "Returns content only if the current browser session has already purchased the item for the selected asset.", "", map[string]any{
					"item": map[string]any{
						"id":          "book-go-101",
						"title":       "Go Systems Handbook",
						"type":        "book",
						"description": "A practical field guide to Go services.",
						"prices":      map[string]any{exampleAsset: "2.50"},
						"content":     "Go Systems Handbook\\n\\nThis sample chapter walks through interfaces and runtime lifecycles.",
					},
				}),
			},
			"/api/v1/balance": map[string]any{
				"get": readOperation("Store", "Store balance summary", "Returns the available off-session balance plus the current user and app allocations for one asset.", "", map[string]any{
					"asset":             exampleAsset,
					"available_balance": "5",
					"user_allocation":   "2.50",
					"app_allocation":    "2.50",
					"session_status":    "open",
					"session_id":        "0xsession",
					"version":           2,
				}),
			},
			"/api/v1/balances": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Balances", "Current off-chain balances for the backend signer.", "", map[string]any{
					"balances": []map[string]any{{"asset": exampleAsset, "balance": "2.5"}},
				}),
			},
			"/api/v1/transactions": map[string]any{
				"get": readOperation("Health / Wallet / Node", "Transactions", "Recent transaction history for the signer.", "", map[string]any{
					"transactions": []map[string]any{{"asset": exampleAsset, "type": "transfer", "amount": "1"}},
				}),
			},
			"/api/v1/channel": map[string]any{
				"get": readOperation("Raw Channel", "Home channel", "Returns the home channel summary for an asset.", "", map[string]any{
					"channel": map[string]any{"asset": exampleAsset, "stateVersion": 4, "status": "open"},
				}),
			},
			"/api/v1/channel/state": map[string]any{
				"get": readOperation("Raw Channel", "Latest state", "Returns the latest state for an asset. Use `only_signed=true` for the latest co-signed state.", "", map[string]any{
					"state": map[string]any{"asset": exampleAsset, "version": 4},
				}),
			},
			"/api/v1/approve": map[string]any{
				"post": writeAccessOperation("Raw Channel", "Approve token", "Approves the locking contract to spend the selected ERC-20.", requestBody("", map[string]any{
					"blockchain_id": exampleChain,
					"asset":         exampleAsset,
					"amount":        "1",
				}), "", map[string]any{"tx_hash": "0xapprove"}),
			},
			"/api/v1/deposit": map[string]any{
				"post": writeAccessOperation("Raw Channel", "Deposit", "Builds and submits the next deposit state.", requestBody("", map[string]any{
					"blockchain_id": exampleChain,
					"asset":         exampleAsset,
					"amount":        "1",
				}), "", map[string]any{
					"ready_for_checkpoint": true,
					"state":                map[string]any{"version": 4, "asset": exampleAsset},
				}),
			},
			"/api/v1/withdraw": map[string]any{
				"post": writeAccessOperation("Raw Channel", "Withdraw", "Builds and submits the next withdrawal state.", requestBody("", map[string]any{
					"blockchain_id": exampleChain,
					"asset":         exampleAsset,
					"amount":        "0.5",
				}), "", map[string]any{
					"ready_for_checkpoint": true,
					"state":                map[string]any{"version": 5, "asset": exampleAsset},
				}),
			},
			"/api/v1/transfer": map[string]any{
				"post": writeAccessOperation("Raw Channel", "Transfer", "Builds and submits an off-chain transfer state.", requestBody("", map[string]any{
					"recipient": "0x1111111111111111111111111111111111111111",
					"asset":     exampleAsset,
					"amount":    "0.25",
				}), "", map[string]any{
					"state": map[string]any{"version": 6, "asset": exampleAsset},
				}),
			},
			"/api/v1/checkpoint": map[string]any{
				"post": writeAccessOperation("Raw Channel", "Checkpoint", "Submits the latest signed state on-chain.", requestBody("", map[string]any{
					"asset": exampleAsset,
				}), "", map[string]any{"tx_hash": "0xcheckpoint"}),
			},
			"/api/v1/challenge": map[string]any{
				"post": writeAccessOperation("Raw Channel", "Challenge latest state", "Challenges the latest signed state for the selected asset.", requestBody("", map[string]any{"asset": exampleAsset}), "", map[string]any{"tx_hash": "0xchallenge"}),
			},
			"/api/v1/channel/close": map[string]any{
				"post": writeAccessOperation("Raw Channel", "Close channel", "Builds the finalize state for the selected asset.", requestBody("", map[string]any{"asset": exampleAsset}), "", map[string]any{"state": map[string]any{"version": 7, "asset": exampleAsset}}),
			},
			"/api/v1/apps": map[string]any{
				"get": readOperation("Raw Sessions", "Apps", "Lists registered apps.", "", map[string]any{"apps": []map[string]any{{"app_id": cfg.StoreAppID, "creation_approval_not_required": true}}}),
			},
			"/api/v1/apps/register": map[string]any{
				"post": writeAccessOperation("Raw Sessions", "Register app", "Registers a new app definition.", requestBody("", map[string]any{
					"app_id":                         "demo-app",
					"metadata":                       "{}",
					"creation_approval_not_required": true,
				}), "", map[string]any{"status": "registered"}),
			},
			"/api/v1/sessions": map[string]any{
				"get": readOperation("Raw Sessions", "Sessions", "Lists sessions for the signer by default.", "", map[string]any{
					"sessions": []map[string]any{{"app_session_id": "0xsession", "application_id": cfg.StoreAppID, "status": "open", "version": 4}},
				}),
				"post": writeAccessOperation("Raw Sessions", "Create session", "Creates a single-signer app session for the backend signer.", requestBody("", map[string]any{
					"application_id": cfg.StoreAppID,
					"initial_allocations": []map[string]any{
						{"asset": exampleAsset, "amount": "0.5"},
					},
					"session_data": "{}",
				}), "", map[string]any{"session_id": "0xsession", "status": "open", "version": 2}),
			},
			"/api/v1/sessions/{session_id}": map[string]any{
				"get": readOperation("Raw Sessions", "Session detail", "Returns the session and app definition for a specific session.", "", map[string]any{
					"session":        map[string]any{"app_session_id": "0xsession", "version": 4},
					"app_definition": map[string]any{"application_id": cfg.StoreAppID},
				}),
			},
			"/api/v1/sessions/{session_id}/deposit": map[string]any{
				"post": writeAccessOperation("Raw Sessions", "Session deposit", "Deposits into an open app session.", requestBody("", map[string]any{
					"asset":  exampleAsset,
					"amount": "0.25",
				}), "", map[string]any{"session_id": "0xsession", "version": 3, "node_sig": "0xnodesig"}),
			},
			"/api/v1/sessions/{session_id}/state": map[string]any{
				"post": writeAccessOperation("Raw Sessions", "Operate session", "Submits an operate update for the selected session.", requestBody("", map[string]any{
					"allocations":  []map[string]any{{"participant": "0xabc", "asset": exampleAsset, "amount": "0.5"}},
					"session_data": "{\"turn\":1}",
				}), "", map[string]any{"session_id": "0xsession", "version": 4}),
			},
			"/api/v1/sessions/{session_id}/close": map[string]any{
				"post": writeAccessOperation("Raw Sessions", "Close session", "Closes the selected session using its current allocations.", requestBody("", map[string]any{}), "", map[string]any{"session_id": "0xsession", "status": "closed"}),
			},
			"/api/v1/session-keys/channel": map[string]any{
				"get": readOperation("Raw Session Keys", "Channel session keys", "Lists active channel session key states.", "", map[string]any{"states": []any{}}),
				"post": writeAccessOperation("Raw Session Keys", "Register channel session key", "Registers a channel session key state.", requestBody("", map[string]any{
					"session_key": "0x1111111111111111111111111111111111111111",
					"assets":      []string{exampleAsset},
					"expires_at":  "2030-01-01T00:00:00Z",
				}), "", map[string]any{"state": map[string]any{"session_key": "0x1111111111111111111111111111111111111111", "version": 1}}),
			},
			"/api/v1/session-keys/app": map[string]any{
				"get": readOperation("Raw Session Keys", "App session keys", "Lists active app session key states.", "", map[string]any{"states": []any{}}),
				"post": writeAccessOperation("Raw Session Keys", "Register app session key", "Registers an app session key state.", requestBody("", map[string]any{
					"session_key":     "0x1111111111111111111111111111111111111111",
					"application_ids": []string{cfg.StoreAppID},
					"app_session_ids": []string{},
					"expires_at":      "2030-01-01T00:00:00Z",
				}), "", map[string]any{"state": map[string]any{"session_key": "0x1111111111111111111111111111111111111111", "version": 1}}),
			},
			"/api/v1/demo/overview": map[string]any{
				"get": readOperation("Legacy", "Legacy demo overview", "Legacy guided-demo aggregate kept for compatibility with the earlier protocol-first UI.", "", map[string]any{
					"selected_asset": exampleAsset,
					"channel_guidance": map[string]any{
						"next_action": "approve_and_deposit",
						"headline":    "Fund the home channel",
					},
				}),
			},
		},
	}
}

func readOperation(tag string, summary string, description string, schemaName string, example any) map[string]any {
	return map[string]any{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"responses":   standardResponses(http.StatusOK, schemaName, example, false),
	}
}

func writeAccessOperation(tag string, summary string, description string, request any, schemaName string, example any) map[string]any {
	op := map[string]any{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"responses":   standardResponses(http.StatusOK, schemaName, example, true),
		"security": []map[string]any{
			{"writeSessionCookie": []string{}},
			{"bearerAuth": []string{}},
		},
	}
	if request != nil {
		op["requestBody"] = request
	}
	return op
}

func operatorLeaseOperation(tag string, summary string, description string, request any, schemaName string, example any, statusCode int) map[string]any {
	op := map[string]any{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"responses":   standardResponses(statusCode, schemaName, example, true),
		"security": []map[string]any{
			{"writeSessionCookie": []string{}},
		},
	}
	if request != nil {
		op["requestBody"] = request
	}
	return op
}

func asyncOperatorOperation(tag string, summary string, description string, requestExample any, responseExample any) map[string]any {
	return map[string]any{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"requestBody": requestBody("", requestExample),
		"security": []map[string]any{
			{"writeSessionCookie": []string{}},
		},
		"responses": standardResponses(http.StatusAccepted, "AsyncOperationAccepted", responseExample, true),
	}
}

func asyncPublicOperation(tag string, summary string, description string, requestExample any, responseExample any) map[string]any {
	return map[string]any{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"requestBody": requestBody("", requestExample),
		"responses":   standardResponses(http.StatusAccepted, "AsyncOperationAccepted", responseExample, true),
	}
}

func leaseOwnershipOperation(tag string, summary string, description string, request any, schemaName string, example any) map[string]any {
	return map[string]any{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"requestBody": request,
		"security": []map[string]any{
			{"writeSessionCookie": []string{}},
		},
		"responses": standardResponses(http.StatusOK, schemaName, example, true),
	}
}

func standardResponses(statusCode int, schemaName string, example any, allowWriteErrors bool) map[string]any {
	statusText := "ok"
	switch statusCode {
	case http.StatusCreated:
		statusText = "created"
	case http.StatusAccepted:
		statusText = "accepted"
	}

	out := map[string]any{
		statusCodeKey(statusCode): map[string]any{
			"description": statusText,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema":  responseSchema(schemaName),
					"example": example,
				},
			},
		},
		"400": errorResponse("invalid request"),
		"422": errorResponse("operation failed"),
		"503": errorResponse("service unavailable"),
	}
	if allowWriteErrors {
		out["409"] = errorResponse("conflict")
		out["401"] = errorResponse("missing or invalid api key")
	}
	return out
}

func statusCodeKey(statusCode int) string {
	switch statusCode {
	case http.StatusCreated:
		return "201"
	case http.StatusAccepted:
		return "202"
	default:
		return "200"
	}
}

func errorResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schemaRef("ErrorEnvelope"),
			},
		},
	}
}

func requestBody(schemaName string, example any) map[string]any {
	schema := genericObjectSchema()
	if schemaName != "" {
		schema = schemaRef(schemaName)
	}
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema":  schema,
				"example": example,
			},
		},
	}
}

func responseSchema(schemaName string) any {
	if schemaName == "" {
		return genericObjectSchema()
	}
	return schemaRef(schemaName)
}

func genericObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func openAPIAssetAndChain(homeBlockchains map[string]uint64) (string, uint64) {
	assets := make([]string, 0, len(homeBlockchains))
	for asset := range homeBlockchains {
		assets = append(assets, asset)
	}
	sort.Strings(assets)
	if len(assets) == 0 {
		return "", 0
	}
	return assets[0], homeBlockchains[assets[0]]
}
