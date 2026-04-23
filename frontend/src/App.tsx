import { useEffect, useMemo, useRef, useState } from 'react'
import Decimal from 'decimal.js'
import {
  NitroliteClient,
  packCreateAppSessionHash,
  packSubmitAppStateHash,
  toWalletQuorumSignature,
} from '@yellow-org/sdk-compat'
import { applyCommitTransition, nextState, type State as ChannelState } from '@yellow-org/sdk'
import { createWalletClient, custom, type Address, type Hex, type WalletClient } from 'viem'
import { sepolia } from 'viem/chains'
import './App.css'

const DEFAULT_ASSET = 'yusd'
const DEFAULT_WS_URL = import.meta.env.VITE_CLEARNODE_WS_URL || 'wss://clearnode-sandbox.yellow.org/v1/ws'
const DEFAULT_BLOCKCHAIN_RPCS: Record<number, string> = {
  11155111: import.meta.env.VITE_BLOCKCHAIN_RPC_11155111 || 'https://ethereum-sepolia-rpc.publicnode.com',
}
const APP_STATE_INTENT = {
  Operate: 0,
  Deposit: 1,
  Withdraw: 2,
} as const

type InjectedProvider = {
  request: (args: { method: string; params?: unknown[] }) => Promise<unknown>
  isMetaMask?: boolean
}

type StoreCatalogItem = {
  id: string
  title: string
  description: string
  type: string
  prices: Record<string, string>
  content?: string
}

type StoreSession = {
  asset: string
  app_session_id?: string
  status: string
  version: number
  user_allocation: string
  app_allocation: string
  session_data?: string
}

type StoreLibraryItem = {
  id: string
  title: string
  description: string
  type: string
  price: string
  purchased_at: string
}

type StoreBootstrap = {
  store_name: string
  app_id: string
  app_signer: string
  wallet_address: string
  selected_asset: string
  default_asset: string
  supported_assets: string[]
  available_balance: string
  catalog: StoreCatalogItem[]
  session: StoreSession
  library: StoreLibraryItem[]
}

type ContentResponse = StoreCatalogItem

type APIError = {
  error?: {
    code: string
    message: string
  }
}

function getMetaMaskProvider(): InjectedProvider | null {
  const ethereum = (window as Window & { ethereum?: InjectedProvider & { providers?: InjectedProvider[] } }).ethereum
  if (!ethereum) return null
  if (Array.isArray((ethereum as InjectedProvider & { providers?: InjectedProvider[] }).providers)) {
    const provider = (ethereum as InjectedProvider & { providers?: InjectedProvider[] }).providers?.find((entry) => entry.isMetaMask)
    return provider ?? null
  }
  return ethereum.isMetaMask ? ethereum : null
}

async function readJSON<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
    ...init,
  })

  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as APIError
    throw new Error(payload.error?.message || `Request failed with status ${response.status}`)
  }

  return response.json() as Promise<T>
}

function shortAddress(value: string | null): string {
  if (!value) return 'Not connected'
  if (value.length <= 12) return value
  return `${value.slice(0, 6)}...${value.slice(-4)}`
}

function formatAmount(value: string): string {
  return new Decimal(value || '0').toFixed()
}

function parseSessionDataLabel(sessionData?: string): string {
  if (!sessionData) return 'No store activity yet.'
  try {
    const parsed = JSON.parse(sessionData) as { action?: string; item_id?: string; amount?: string; price?: string }
    switch (parsed.action) {
      case 'bootstrap':
        return 'Store session ready.'
      case 'deposit':
        return `Last action: added ${parsed.amount}`
      case 'purchase':
        return `Last action: purchased ${parsed.item_id} for ${parsed.price}`
      case 'user_withdraw':
        return `Last action: withdrew ${parsed.amount}`
      default:
        return 'Store session active.'
    }
  } catch {
    return 'Store session active.'
  }
}

function toRPCState(state: ChannelState) {
  return {
    id: state.id,
    transition: {
      type: state.transition.type,
      tx_id: state.transition.txId,
      account_id: state.transition.accountId ?? '',
      amount: state.transition.amount.toString(),
    },
    asset: state.asset,
    user_wallet: state.userWallet,
    epoch: state.epoch.toString(),
    version: state.version.toString(),
    home_channel_id: state.homeChannelId ?? undefined,
    escrow_channel_id: state.escrowChannelId ?? undefined,
    home_ledger: {
      token_address: state.homeLedger.tokenAddress,
      blockchain_id: state.homeLedger.blockchainId.toString(),
      user_balance: state.homeLedger.userBalance.toString(),
      user_net_flow: state.homeLedger.userNetFlow.toString(),
      node_balance: state.homeLedger.nodeBalance.toString(),
      node_net_flow: state.homeLedger.nodeNetFlow.toString(),
    },
    escrow_ledger: state.escrowLedger
      ? {
          token_address: state.escrowLedger.tokenAddress,
          blockchain_id: state.escrowLedger.blockchainId.toString(),
          user_balance: state.escrowLedger.userBalance.toString(),
          user_net_flow: state.escrowLedger.userNetFlow.toString(),
          node_balance: state.escrowLedger.nodeBalance.toString(),
          node_net_flow: state.escrowLedger.nodeNetFlow.toString(),
        }
      : undefined,
    user_sig: state.userSig,
    node_sig: state.nodeSig,
  }
}

export default function App() {
  const [walletAddress, setWalletAddress] = useState<string | null>(null)
  const [walletClient, setWalletClient] = useState<WalletClient | null>(null)
  const [nitroliteClient, setNitroliteClient] = useState<NitroliteClient | null>(null)
  const [selectedAsset, setSelectedAsset] = useState(DEFAULT_ASSET)
  const [bootstrap, setBootstrap] = useState<StoreBootstrap | null>(null)
  const [depositAmount, setDepositAmount] = useState('1.00')
  const [withdrawAmount, setWithdrawAmount] = useState('0.50')
  const [readerItem, setReaderItem] = useState<ContentResponse | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [activity, setActivity] = useState<string[]>([])
  const autoCreatingRef = useRef(false)

  const libraryIds = useMemo(() => new Set((bootstrap?.library ?? []).map((item) => item.id)), [bootstrap])

  function appendLog(line: string) {
    const stamped = `${new Date().toISOString()} ${line}`
    setActivity((current) => [stamped, ...current].slice(0, 40))
  }

  async function ensureSepolia(provider: InjectedProvider) {
    const chainId = (await provider.request({ method: 'eth_chainId' })) as string
    if (chainId === '0xaa36a7') return

    try {
      await provider.request({
        method: 'wallet_switchEthereumChain',
        params: [{ chainId: '0xaa36a7' }],
      })
    } catch (switchError) {
      throw new Error(`Switch MetaMask to Sepolia and try again. ${String(switchError)}`)
    }
  }

  async function refreshBootstrap(asset: string, clientOverride?: NitroliteClient | null, walletOverride?: string | null, walletClientOverride?: WalletClient | null) {
    const client = clientOverride ?? nitroliteClient
    const wallet = walletOverride ?? walletAddress
    const clientSigner = walletClientOverride ?? walletClient
    const next = await readJSON<StoreBootstrap>(`/api/store/bootstrap?asset=${encodeURIComponent(asset)}`)
    setBootstrap(next)
    setSelectedAsset(next.selected_asset)
    appendLog(`bootstrap ${asset} -> ${next.session.status}`)

    if (next.session.status === 'missing' && client && wallet && clientSigner && !autoCreatingRef.current) {
      autoCreatingRef.current = true
      try {
        const created = await createSession(next, clientSigner, wallet)
        setBootstrap(created)
        appendLog(`session created ${created.session.app_session_id}`)
      } finally {
        autoCreatingRef.current = false
      }
    }
  }

  async function connectWallet() {
    setBusy('connect')
    setError(null)

    try {
      const provider = getMetaMaskProvider()
      if (!provider) {
        throw new Error('MetaMask extension was not found in this browser.')
      }

      await ensureSepolia(provider)
      const accounts = (await provider.request({ method: 'eth_requestAccounts' })) as string[]
      const wallet = accounts[0]
      if (!wallet) {
        throw new Error('MetaMask did not return an account.')
      }

      const client = createWalletClient({
        account: wallet as Address,
        chain: sepolia,
        transport: custom(provider),
      })

      const challenge = await readJSON<{ challenge_id: string; message: string }>('/api/store/connect/challenge', {
        method: 'POST',
        body: JSON.stringify({ wallet_address: wallet }),
      })
      const rawSignature = await client.signMessage({
        account: wallet as Address,
        message: challenge.message,
      })

      await readJSON('/api/store/connect/verify', {
        method: 'POST',
        body: JSON.stringify({
          challenge_id: challenge.challenge_id,
          wallet_address: wallet,
          signature: rawSignature,
        }),
      })

      const nitrolite = await NitroliteClient.create({
        wsURL: DEFAULT_WS_URL,
        walletClient: client,
        chainId: 11155111,
        blockchainRPCs: DEFAULT_BLOCKCHAIN_RPCS,
      })

      setWalletAddress(wallet)
      setWalletClient(client)
      setNitroliteClient(nitrolite)
      appendLog(`wallet connected ${wallet}`)
      await refreshBootstrap(selectedAsset, nitrolite, wallet, client)
    } catch (connectError) {
      setError(connectError instanceof Error ? connectError.message : 'Failed to connect wallet')
    } finally {
      setBusy(null)
    }
  }

  async function createSession(nextBootstrap: StoreBootstrap, client: WalletClient, wallet: string) {
    const nonce = Date.now()
    const sessionData = JSON.stringify({ action: 'bootstrap', asset: nextBootstrap.selected_asset })
    const definition = {
      application_id: nextBootstrap.app_id,
      participants: [
        { wallet_address: wallet, signature_weight: 1 },
        { wallet_address: nextBootstrap.app_signer, signature_weight: 1 },
      ],
      quorum: 2,
      nonce: String(nonce),
    }

    const payloadHash = packCreateAppSessionHash({
      application: nextBootstrap.app_id,
      participants: definition.participants.map((participant) => ({
        walletAddress: participant.wallet_address as Address,
        signatureWeight: participant.signature_weight,
      })),
      quorum: definition.quorum,
      nonce,
      sessionData,
    })

    const rawSignature = await client.signMessage({
      account: wallet as Address,
      message: { raw: payloadHash },
    })
    const userSignature = toWalletQuorumSignature(rawSignature as Hex)

    return readJSON<StoreBootstrap>('/api/store/update', {
      method: 'POST',
      body: JSON.stringify({
        asset: nextBootstrap.selected_asset,
        kind: 'create_session',
        definition,
        session_data: sessionData,
        user_signature: userSignature,
      }),
    })
  }

  async function submitPurchase(item: StoreCatalogItem) {
    if (!bootstrap || !walletAddress || !walletClient) return
    if (!bootstrap.session.app_session_id || bootstrap.session.status !== 'open') {
      setError('Store session is not ready yet.')
      return
    }

    setBusy(`purchase:${item.id}`)
    setError(null)
    try {
      const version = bootstrap.session.version + 1
      const currentUser = new Decimal(bootstrap.session.user_allocation)
      const currentApp = new Decimal(bootstrap.session.app_allocation)
      const price = new Decimal(item.prices[bootstrap.selected_asset])
      const nextUser = currentUser.minus(price)
      const nextApp = currentApp.plus(price)
      if (nextUser.isNegative()) {
        throw new Error('Add funds before purchasing this item.')
      }

      const sessionData = JSON.stringify({
        action: 'purchase',
        item_id: item.id,
        price: price.toFixed(),
      })
      const appStateUpdate = {
        app_session_id: bootstrap.session.app_session_id,
        intent: APP_STATE_INTENT.Operate,
        version: String(version),
        allocations: [
          { participant: walletAddress as Address, asset: bootstrap.selected_asset, amount: nextUser.toFixed() },
          { participant: bootstrap.app_signer as Address, asset: bootstrap.selected_asset, amount: nextApp.toFixed() },
        ],
        session_data: sessionData,
      }

      const payloadHash = packSubmitAppStateHash({
        appSessionId: appStateUpdate.app_session_id as Hex,
        intent: appStateUpdate.intent,
        version,
        allocations: appStateUpdate.allocations,
        sessionData,
      })

      const rawSignature = await walletClient.signMessage({
        account: walletAddress as Address,
        message: { raw: payloadHash },
      })
      const userSignature = toWalletQuorumSignature(rawSignature)

      const next = await readJSON<StoreBootstrap>('/api/store/update', {
        method: 'POST',
        body: JSON.stringify({
          asset: bootstrap.selected_asset,
          kind: 'submit_app_state',
          app_state_update: appStateUpdate,
          user_signature: userSignature,
        }),
      })
      setBootstrap(next)
      appendLog(`purchase ${item.id}`)
    } catch (purchaseError) {
      setError(purchaseError instanceof Error ? purchaseError.message : 'Failed to buy item')
    } finally {
      setBusy(null)
    }
  }

  async function submitWithdraw() {
    if (!bootstrap || !walletAddress || !walletClient) return
    if (!bootstrap.session.app_session_id || bootstrap.session.status !== 'open') {
      setError('Store session is not ready yet.')
      return
    }

    setBusy('withdraw')
    setError(null)
    try {
      const amount = new Decimal(withdrawAmount)
      const version = bootstrap.session.version + 1
      const currentUser = new Decimal(bootstrap.session.user_allocation)
      const currentApp = new Decimal(bootstrap.session.app_allocation)
      const nextUser = currentUser.minus(amount)
      if (nextUser.isNegative()) {
        throw new Error('Withdraw amount exceeds your store balance.')
      }

      const sessionData = JSON.stringify({
        action: 'user_withdraw',
        amount: amount.toFixed(),
      })
      const appStateUpdate = {
        app_session_id: bootstrap.session.app_session_id,
        intent: APP_STATE_INTENT.Withdraw,
        version: String(version),
        allocations: [
          { participant: walletAddress as Address, asset: bootstrap.selected_asset, amount: nextUser.toFixed() },
          { participant: bootstrap.app_signer as Address, asset: bootstrap.selected_asset, amount: currentApp.toFixed() },
        ],
        session_data: sessionData,
      }

      const payloadHash = packSubmitAppStateHash({
        appSessionId: appStateUpdate.app_session_id as Hex,
        intent: appStateUpdate.intent,
        version,
        allocations: appStateUpdate.allocations,
        sessionData,
      })
      const rawSignature = await walletClient.signMessage({
        account: walletAddress as Address,
        message: { raw: payloadHash },
      })

      const next = await readJSON<StoreBootstrap>('/api/store/update', {
        method: 'POST',
        body: JSON.stringify({
          asset: bootstrap.selected_asset,
          kind: 'submit_app_state',
          app_state_update: appStateUpdate,
          user_signature: toWalletQuorumSignature(rawSignature),
        }),
      })
      setBootstrap(next)
      appendLog(`withdraw ${amount.toFixed()}`)
    } catch (withdrawError) {
      setError(withdrawError instanceof Error ? withdrawError.message : 'Failed to withdraw')
    } finally {
      setBusy(null)
    }
  }

  async function submitDeposit() {
    if (!bootstrap || !walletAddress || !walletClient || !nitroliteClient) return
    if (!bootstrap.session.app_session_id || bootstrap.session.status !== 'open') {
      setError('Store session is not ready yet.')
      return
    }

    setBusy('deposit')
    setError(null)
    try {
      const amount = new Decimal(depositAmount)
      const version = bootstrap.session.version + 1
      const currentUser = new Decimal(bootstrap.session.user_allocation)
      const currentApp = new Decimal(bootstrap.session.app_allocation)
      const nextUser = currentUser.plus(amount)

      const sessionData = JSON.stringify({
        action: 'deposit',
        amount: amount.toFixed(),
      })
      const appStateUpdate = {
        app_session_id: bootstrap.session.app_session_id,
        intent: APP_STATE_INTENT.Deposit,
        version: String(version),
        allocations: [
          { participant: walletAddress as Address, asset: bootstrap.selected_asset, amount: nextUser.toFixed() },
          { participant: bootstrap.app_signer as Address, asset: bootstrap.selected_asset, amount: currentApp.toFixed() },
        ],
        session_data: sessionData,
      }

      const payloadHash = packSubmitAppStateHash({
        appSessionId: appStateUpdate.app_session_id as Hex,
        intent: appStateUpdate.intent,
        version,
        allocations: appStateUpdate.allocations,
        sessionData,
      })
      const rawSignature = await walletClient.signMessage({
        account: walletAddress as Address,
        message: { raw: payloadHash },
      })
      const userSignature = toWalletQuorumSignature(rawSignature)

      const currentState = await nitroliteClient.innerClient.getLatestState(walletAddress as Address, bootstrap.selected_asset, false)
      const proposedState = nextState(currentState)
      applyCommitTransition(proposedState, bootstrap.session.app_session_id, amount)
      proposedState.userSig = await nitroliteClient.innerClient.signState(proposedState)

      const next = await readJSON<StoreBootstrap>('/api/store/update', {
        method: 'POST',
        body: JSON.stringify({
          asset: bootstrap.selected_asset,
          kind: 'submit_deposit_state',
          app_state_update: appStateUpdate,
          user_signature: userSignature,
          user_state: toRPCState(proposedState),
        }),
      })
      setBootstrap(next)
      appendLog(`deposit ${amount.toFixed()}`)
    } catch (depositError) {
      setError(depositError instanceof Error ? depositError.message : 'Failed to add funds')
    } finally {
      setBusy(null)
    }
  }

  async function openContent(itemID: string) {
    if (!bootstrap) return
    setBusy(`content:${itemID}`)
    setError(null)
    try {
      const item = await readJSON<ContentResponse>(`/api/store/content/${encodeURIComponent(itemID)}?asset=${encodeURIComponent(bootstrap.selected_asset)}`)
      setReaderItem(item)
      appendLog(`open content ${itemID}`)
    } catch (contentError) {
      setError(contentError instanceof Error ? contentError.message : 'Failed to open content')
    } finally {
      setBusy(null)
    }
  }

  async function copyActivity() {
    try {
      await navigator.clipboard.writeText(activity.join('\n'))
      appendLog('copied activity log')
    } catch (copyError) {
      setError(copyError instanceof Error ? copyError.message : 'Failed to copy activity log')
    }
  }

  useEffect(() => {
    if (!bootstrap || !walletAddress || !nitroliteClient) return
    if (bootstrap.selected_asset === selectedAsset) return
    void refreshBootstrap(selectedAsset)
  }, [selectedAsset, bootstrap, walletAddress, nitroliteClient])

  return (
    <main className="shell">
      <section className="hero">
        <div>
          <p className="eyebrow">Content store</p>
          <h1>Simple content store</h1>
          <p className="lede">
            Connect MetaMask, add funds, buy content instantly, read what you own, and withdraw what remains.
          </p>
        </div>
        <div className="hero-actions">
          <button className="primary" onClick={connectWallet} disabled={busy === 'connect'}>
            {busy === 'connect' ? 'Connecting…' : walletAddress ? 'Reconnect MetaMask' : 'Connect MetaMask'}
          </button>
          <div className="identity-card">
            <span className="label">Wallet</span>
            <strong>{shortAddress(walletAddress)}</strong>
          </div>
        </div>
      </section>

      {error ? <div className="alert">{error}</div> : null}

      <section className="grid two-up">
        <article className="panel">
          <div className="panel-head">
            <div>
              <span className="label">Store</span>
              <h2>{bootstrap?.store_name ?? 'Store'}</h2>
            </div>
            <label className="asset-picker">
              <span>Asset</span>
              <select value={selectedAsset} onChange={(event) => setSelectedAsset(event.target.value)} disabled={!walletAddress}>
                {(bootstrap?.supported_assets ?? [DEFAULT_ASSET]).map((asset) => (
                  <option key={asset} value={asset}>
                    {asset.toUpperCase()}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <div className="stats">
            <div className="stat">
              <span className="label">Available balance</span>
              <strong>{bootstrap ? formatAmount(bootstrap.available_balance) : '0'}</strong>
            </div>
            <div className="stat">
              <span className="label">Store balance</span>
              <strong>{bootstrap ? formatAmount(bootstrap.session.user_allocation) : '0'}</strong>
            </div>
            <div className="stat">
              <span className="label">Store status</span>
              <strong>{bootstrap?.session.status ?? 'Connect first'}</strong>
            </div>
          </div>

          <p className="supporting">{bootstrap ? parseSessionDataLabel(bootstrap.session.session_data) : 'Connect MetaMask to start shopping.'}</p>

          <div className="form-grid">
            <label>
              <span>Add funds</span>
              <input value={depositAmount} onChange={(event) => setDepositAmount(event.target.value)} inputMode="decimal" />
            </label>
            <button className="primary" disabled={!walletAddress || busy === 'deposit'} onClick={submitDeposit}>
              {busy === 'deposit' ? 'Adding…' : 'Add funds'}
            </button>
            <label>
              <span>Withdraw</span>
              <input value={withdrawAmount} onChange={(event) => setWithdrawAmount(event.target.value)} inputMode="decimal" />
            </label>
            <button className="secondary" disabled={!walletAddress || busy === 'withdraw'} onClick={submitWithdraw}>
              {busy === 'withdraw' ? 'Withdrawing…' : 'Withdraw to wallet'}
            </button>
          </div>
        </article>

        <article className="panel">
          <div className="panel-head">
            <div>
              <span className="label">Library</span>
              <h2>What you own</h2>
            </div>
          </div>

          {bootstrap?.library.length ? (
            <div className="stack">
              {bootstrap.library.map((item) => (
                <div className="library-item" key={item.id}>
                  <div>
                    <strong>{item.title}</strong>
                    <p>{item.description}</p>
                    <span className="meta">{item.type} · bought for {item.price}</span>
                  </div>
                  <button className="secondary" disabled={busy === `content:${item.id}`} onClick={() => openContent(item.id)}>
                    {busy === `content:${item.id}` ? 'Opening…' : 'Read'}
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <p className="supporting">Buy something from the catalog and it will appear here.</p>
          )}
        </article>
      </section>

      <section className="grid two-up">
        <article className="panel">
          <div className="panel-head">
            <div>
              <span className="label">Catalog</span>
              <h2>Browse content</h2>
            </div>
          </div>

          <div className="stack">
            {(bootstrap?.catalog ?? []).map((item) => (
              <div className="catalog-item" key={item.id}>
                <div>
                  <strong>{item.title}</strong>
                  <p>{item.description}</p>
                  <span className="meta">{item.type} · {item.prices[selectedAsset]}</span>
                </div>
                <button className="primary" disabled={!walletAddress || libraryIds.has(item.id) || busy === `purchase:${item.id}`} onClick={() => submitPurchase(item)}>
                  {libraryIds.has(item.id) ? 'Owned' : busy === `purchase:${item.id}` ? 'Buying…' : 'Buy'}
                </button>
              </div>
            ))}
          </div>
        </article>

        <article className="panel">
          <div className="panel-head">
            <div>
              <span className="label">Reader</span>
              <h2>Open content</h2>
            </div>
          </div>

          {readerItem ? (
            <article className="reader">
              <h3>{readerItem.title}</h3>
              <pre>{readerItem.content}</pre>
            </article>
          ) : (
            <p className="supporting">Open an item from your library to read it here.</p>
          )}
        </article>
      </section>

      <section className="panel">
        <div className="panel-head">
          <div>
            <span className="label">Browser activity</span>
            <h2>Recent actions</h2>
          </div>
          <button className="secondary" onClick={copyActivity} disabled={activity.length === 0}>
            Copy log
          </button>
        </div>

        {activity.length ? (
          <ul className="activity-list">
            {activity.map((entry) => (
              <li key={entry}>{entry}</li>
            ))}
          </ul>
        ) : (
          <p className="supporting">No activity yet.</p>
        )}
      </section>
    </main>
  )
}
