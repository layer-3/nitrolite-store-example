import { type ReactNode, useMemo, useRef, useState } from 'react'
import Decimal from 'decimal.js'
import { AnimatePresence, motion, type HTMLMotionProps } from 'motion/react'
import { Activity, BookOpen, CheckCircle2, Copy, CreditCard, Library, Loader2, RefreshCw, ShieldCheck, ShoppingBag, Wallet } from 'lucide-react'
import { clsx, type ClassValue } from 'clsx'
import {
  AppSessionWalletSignerV1,
  AppStateUpdateIntent,
  ChannelDefaultSigner,
  Client,
  packAppStateUpdateV1,
  packCreateAppSessionRequestV1,
  withBlockchainRPC,
  type AppDefinitionV1,
  type AppStateUpdateV1,
  type StateSigner,
  type TransactionSigner,
} from '@yellow-org/sdk'
import { createWalletClient, custom, type Address, type Hex, type WalletClient } from 'viem'
import { sepolia } from 'viem/chains'
import { twMerge } from 'tailwind-merge'
import './App.css'

const DEFAULT_ASSET = 'yusd'
const DEMO_ASSETS = ['yusd', 'yellow']
const DEFAULT_WS_URL = import.meta.env.VITE_CLEARNODE_WS_URL || 'wss://clearnode-sandbox.yellow.org/v1/ws'
const DEFAULT_BLOCKCHAIN_RPCS: Record<number, string> = {
  11155111: import.meta.env.VITE_BLOCKCHAIN_RPC_11155111 || 'https://ethereum-sepolia-rpc.publicnode.com',
}

type InjectedProvider = {
  request: (args: { method: string; params?: unknown[] }) => Promise<unknown>
  isMetaMask?: boolean
}

class BrowserWalletSigner implements StateSigner, TransactionSigner {
  private readonly client: WalletClient
  private readonly account: Address

  constructor(client: WalletClient, account: Address) {
    this.client = client
    this.account = account
  }

  getAddress(): Address {
    return this.account
  }

  async signMessage(message: Hex | { raw: Hex }): Promise<Hex> {
    const raw = typeof message === 'string' ? message : message.raw
    return this.client.signMessage({
      account: this.account,
      message: { raw },
    })
  }

  async signPersonalMessage(hash: Hex): Promise<Hex> {
    return this.signMessage(hash)
  }

  async sendTransaction(tx: Parameters<WalletClient['sendTransaction']>[0]): Promise<Hex> {
    const rest = { ...tx } as Record<string, unknown>
    delete rest.account
    delete rest.chain
    return this.client.sendTransaction({
      ...rest,
      account: this.account,
      chain: sepolia,
    } as Parameters<WalletClient['sendTransaction']>[0])
  }
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

type StoreUpdateResponse = {
  status: string
  intent: string
  asset: string
  app_session_id: string
  app_signature?: Hex
  bootstrap?: StoreBootstrap
}

type APIError = {
  error?: {
    code: string
    message: string
  }
}

function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
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

function isPositiveAmount(value: string): boolean {
  try {
    return new Decimal(value).isPositive()
  } catch {
    return false
  }
}

function parseSessionDataLabel(sessionData?: string): string {
  if (!sessionData) return 'No store activity yet.'
  try {
    const parsed = JSON.parse(sessionData) as { intent?: string; item_id?: string | number; amount?: string; item_price?: string }
    switch (parsed.intent) {
      case 'init':
        return 'Store session ready.'
      case 'user_deposit':
        return 'Last action: deposit signed.'
      case 'purchase':
        return `Last action: purchased ${parsed.item_id} for ${parsed.item_price}`
      case 'user_withdraw':
        return 'Last action: withdrew from store.'
      default:
        return 'Store session active.'
    }
  } catch {
    return 'Store session active.'
  }
}

function toRPCDefinition(definition: AppDefinitionV1) {
  return {
    application_id: definition.applicationId,
    participants: definition.participants.map((participant) => ({
      wallet_address: participant.walletAddress,
      signature_weight: participant.signatureWeight,
    })),
    quorum: definition.quorum,
    nonce: definition.nonce.toString(),
  }
}

function toRPCAppStateUpdate(update: AppStateUpdateV1) {
  return {
    app_session_id: update.appSessionId,
    intent: update.intent,
    version: update.version.toString(),
    allocations: update.allocations.map((allocation) => ({
      participant: allocation.participant,
      asset: allocation.asset,
      amount: allocation.amount.toString(),
    })),
    session_data: update.sessionData,
  }
}

function MagicPanel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <motion.section
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.26, ease: 'easeOut' }}
      className={cn('glass-panel shine-border min-w-0 rounded-lg p-5 sm:p-6', className)}
    >
      {children}
    </motion.section>
  )
}

function ActionButton({ children, className, variant = 'primary', icon, ...props }: Omit<HTMLMotionProps<'button'>, 'children'> & {
  children: ReactNode
  variant?: 'primary' | 'secondary' | 'ghost'
  icon?: ReactNode
}) {
  return (
    <motion.button
      whileHover={props.disabled ? undefined : { scale: 1.02 }}
      whileTap={props.disabled ? undefined : { scale: 0.97 }}
      transition={{ type: 'spring', stiffness: 420, damping: 20 }}
      className={cn(
        'inline-flex min-h-11 items-center justify-center gap-2 rounded-md px-4 text-sm font-black transition disabled:cursor-not-allowed disabled:opacity-50',
        variant === 'primary' && 'shimmer-button bg-ink text-yellow-brand shadow-glow',
        variant === 'secondary' && 'border border-black/10 bg-white text-ink shadow-sm hover:border-black/30',
        variant === 'ghost' && 'bg-transparent text-ink hover:bg-black/5',
        className,
      )}
      {...props}
    >
      {icon}
      <span>{children}</span>
    </motion.button>
  )
}

function NumberTicker({ value }: { value: string }) {
  return (
    <AnimatePresence mode="wait">
      <motion.strong
        key={value}
        initial={{ opacity: 0, y: 5 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -5 }}
        transition={{ duration: 0.18 }}
        className="block max-w-full truncate text-xl font-black tabular-nums text-ink"
      >
        {formatAmount(value)}
      </motion.strong>
    </AnimatePresence>
  )
}

function PanelHeader({ label, title, icon }: { label: string; title: string; icon: ReactNode }) {
  return (
    <div className="mb-5 flex min-w-0 items-start justify-between gap-4">
      <div className="min-w-0">
        <p className="label-text">{label}</p>
        <h2 className="mt-1 break-words text-xl font-black tracking-normal text-ink sm:text-2xl">{title}</h2>
      </div>
      <div className="grid size-10 shrink-0 place-items-center rounded-lg border border-black/10 bg-yellow-brand text-ink shadow-sm">
        {icon}
      </div>
    </div>
  )
}

export default function App() {
  const [walletAddress, setWalletAddress] = useState<string | null>(null)
  const [walletClient, setWalletClient] = useState<WalletClient | null>(null)
  const [nitroliteClient, setNitroliteClient] = useState<Client | null>(null)
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
  const assetOptions = useMemo(() => {
    const supported = new Set(bootstrap?.supported_assets ?? DEMO_ASSETS)
    return DEMO_ASSETS.filter((asset) => supported.has(asset))
  }, [bootstrap])
  const sessionReady = Boolean(bootstrap?.session.app_session_id && bootstrap.session.status === 'open')
  const canDeposit = Boolean(walletAddress && sessionReady && busy === null && isPositiveAmount(depositAmount))
  const canWithdraw = Boolean(walletAddress && sessionReady && busy === null && isPositiveAmount(withdrawAmount))

  function appendLog(line: string) {
    const stamped = `${new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })} ${line}`
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

  async function refreshBootstrap(asset: string, clientOverride?: Client | null, walletOverride?: string | null, walletClientOverride?: WalletClient | null) {
    const client = clientOverride ?? nitroliteClient
    const wallet = walletOverride ?? walletAddress
    const clientSigner = walletClientOverride ?? walletClient
    if (!wallet) {
      throw new Error('Connect a wallet before bootstrapping the store.')
    }
    const next = await readJSON<StoreBootstrap>(
      `/api/store/bootstrap?asset=${encodeURIComponent(asset)}&wallet_address=${encodeURIComponent(wallet)}`,
    )
    setBootstrap(next)
    setSelectedAsset(next.selected_asset)
    appendLog(`bootstrap ${asset.toUpperCase()} -> ${next.session.status}`)

    if (next.session.status === 'missing' && client && wallet && clientSigner && !autoCreatingRef.current) {
      autoCreatingRef.current = true
      try {
        const created = await createSession(next, clientSigner, wallet)
        setBootstrap(created)
        appendLog(`session created ${shortAddress(created.session.app_session_id ?? '')}`)
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

      const walletSigner = new BrowserWalletSigner(client, wallet as Address)
      const nitrolite = await Client.create(
        DEFAULT_WS_URL,
        new ChannelDefaultSigner(walletSigner),
        walletSigner,
        withBlockchainRPC(11155111n, DEFAULT_BLOCKCHAIN_RPCS[11155111]),
      )

      setWalletAddress(wallet)
      setWalletClient(client)
      setNitroliteClient(nitrolite)
      appendLog(`wallet connected ${shortAddress(wallet)}`)
      await refreshBootstrap(selectedAsset, nitrolite, wallet, client)
    } catch (connectError) {
      setError(connectError instanceof Error ? connectError.message : 'Failed to connect wallet')
    } finally {
      setBusy(null)
    }
  }

  async function createSession(nextBootstrap: StoreBootstrap, client: WalletClient, wallet: string) {
    const signer = new AppSessionWalletSignerV1(new BrowserWalletSigner(client, wallet as Address))
    const sessionData = JSON.stringify({ intent: 'init' })
    const definition: AppDefinitionV1 = {
      applicationId: nextBootstrap.app_id,
      participants: [
        { walletAddress: wallet as Address, signatureWeight: 1 },
        { walletAddress: nextBootstrap.app_signer as Address, signatureWeight: 1 },
      ],
      quorum: 2,
      nonce: BigInt(Date.now() * 1000000),
    }

    const payload = packCreateAppSessionRequestV1(definition, sessionData)
    const userSignature = await signer.signMessage(payload)

    return readJSON<StoreBootstrap>('/api/store/init', {
      method: 'POST',
      body: JSON.stringify({
        wallet_address: wallet,
        asset: nextBootstrap.selected_asset,
        definition: toRPCDefinition(definition),
        session_data: sessionData,
        user_signature: userSignature,
      }),
    })
  }

  async function submitPurchase(item: StoreCatalogItem) {
    if (!bootstrap || !walletAddress || !walletClient) return
    if (!sessionReady) {
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
        intent: 'purchase',
        item_id: Number.isFinite(Number(item.id)) ? Number(item.id) : item.id,
        item_price: price.toFixed(),
      })
      const appStateUpdate: AppStateUpdateV1 = {
        appSessionId: bootstrap.session.app_session_id!,
        intent: AppStateUpdateIntent.Operate,
        version: BigInt(version),
        allocations: [
          { participant: walletAddress as Address, asset: bootstrap.selected_asset, amount: nextUser },
          { participant: bootstrap.app_signer as Address, asset: bootstrap.selected_asset, amount: nextApp },
        ],
        sessionData,
      }

      const payload = packAppStateUpdateV1(appStateUpdate)
      const userSignature = await new AppSessionWalletSignerV1(new BrowserWalletSigner(walletClient, walletAddress as Address)).signMessage(payload)

      const result = await readJSON<StoreUpdateResponse>('/api/store/update', {
        method: 'POST',
        body: JSON.stringify({
          asset: bootstrap.selected_asset,
          app_state_update: toRPCAppStateUpdate(appStateUpdate),
          user_signature: userSignature,
        }),
      })
      if (!result.bootstrap) throw new Error('Purchase succeeded but bootstrap was not returned.')
      setBootstrap(result.bootstrap)
      appendLog(`purchase ${item.id}`)
    } catch (purchaseError) {
      setError(purchaseError instanceof Error ? purchaseError.message : 'Failed to buy item')
    } finally {
      setBusy(null)
    }
  }

  async function submitWithdraw() {
    if (!bootstrap || !walletAddress || !walletClient) return
    if (!sessionReady) {
      setError('Store session is not ready yet.')
      return
    }

    setBusy('withdraw')
    setError(null)
    try {
      const amount = new Decimal(withdrawAmount)
      if (!amount.isPositive()) {
        throw new Error('Withdraw amount must be greater than zero.')
      }
      const version = bootstrap.session.version + 1
      const currentUser = new Decimal(bootstrap.session.user_allocation)
      const currentApp = new Decimal(bootstrap.session.app_allocation)
      const nextUser = currentUser.minus(amount)
      if (nextUser.isNegative()) {
        throw new Error('Withdraw amount exceeds your store balance.')
      }

      const sessionData = JSON.stringify({ intent: 'user_withdraw' })
      const appStateUpdate: AppStateUpdateV1 = {
        appSessionId: bootstrap.session.app_session_id!,
        intent: AppStateUpdateIntent.Withdraw,
        version: BigInt(version),
        allocations: [
          { participant: walletAddress as Address, asset: bootstrap.selected_asset, amount: nextUser },
          { participant: bootstrap.app_signer as Address, asset: bootstrap.selected_asset, amount: currentApp },
        ],
        sessionData,
      }

      const payload = packAppStateUpdateV1(appStateUpdate)
      const userSignature = await new AppSessionWalletSignerV1(new BrowserWalletSigner(walletClient, walletAddress as Address)).signMessage(payload)

      const result = await readJSON<StoreUpdateResponse>('/api/store/update', {
        method: 'POST',
        body: JSON.stringify({
          asset: bootstrap.selected_asset,
          app_state_update: toRPCAppStateUpdate(appStateUpdate),
          user_signature: userSignature,
        }),
      })
      if (!result.bootstrap) throw new Error('Withdraw succeeded but bootstrap was not returned.')
      setBootstrap(result.bootstrap)
      appendLog(`withdraw ${amount.toFixed()}`)
    } catch (withdrawError) {
      setError(withdrawError instanceof Error ? withdrawError.message : 'Failed to withdraw')
    } finally {
      setBusy(null)
    }
  }

  async function submitDeposit() {
    if (!bootstrap || !walletAddress || !walletClient || !nitroliteClient) return
    if (!sessionReady) {
      setError('Store session is not ready yet.')
      return
    }

    setBusy('deposit')
    setError(null)
    try {
      const amount = new Decimal(depositAmount)
      if (!amount.isPositive()) {
        throw new Error('Deposit amount must be greater than zero.')
      }
      const version = bootstrap.session.version + 1
      const currentUser = new Decimal(bootstrap.session.user_allocation)
      const currentApp = new Decimal(bootstrap.session.app_allocation)
      const nextUser = currentUser.plus(amount)

      const sessionData = JSON.stringify({ intent: 'user_deposit' })
      const appStateUpdate: AppStateUpdateV1 = {
        appSessionId: bootstrap.session.app_session_id!,
        intent: AppStateUpdateIntent.Deposit,
        version: BigInt(version),
        allocations: [
          { participant: walletAddress as Address, asset: bootstrap.selected_asset, amount: nextUser },
          { participant: bootstrap.app_signer as Address, asset: bootstrap.selected_asset, amount: currentApp },
        ],
        sessionData,
      }

      const payload = packAppStateUpdateV1(appStateUpdate)
      const userSignature = await new AppSessionWalletSignerV1(new BrowserWalletSigner(walletClient, walletAddress as Address)).signMessage(payload)

      const result = await readJSON<StoreUpdateResponse>('/api/store/update', {
        method: 'POST',
        body: JSON.stringify({
          asset: bootstrap.selected_asset,
          app_state_update: toRPCAppStateUpdate(appStateUpdate),
          user_signature: userSignature,
        }),
      })
      if (!result.app_signature) throw new Error('Backend did not return an app signature for deposit.')
      await nitroliteClient.submitAppSessionDeposit(appStateUpdate, [userSignature, result.app_signature], bootstrap.selected_asset, amount)
      await refreshBootstrap(bootstrap.selected_asset)
      appendLog(`deposit ${amount.toFixed()}`)
    } catch (depositError) {
      setError(depositError instanceof Error ? depositError.message : 'Failed to add funds')
    } finally {
      setBusy(null)
    }
  }

  async function openContent(itemID: string) {
    if (!bootstrap || !walletAddress) return
    if (!bootstrap.session.app_session_id) {
      setError('Store session is not ready yet.')
      return
    }

    setBusy(`content:${itemID}`)
    setError(null)
    try {
      const params = new URLSearchParams({
        wallet_address: walletAddress,
        asset: bootstrap.selected_asset,
      })
      const item = await readJSON<ContentResponse>(`/api/store/content/${encodeURIComponent(itemID)}?${params.toString()}`)
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

  function switchAsset(asset: string) {
    setReaderItem(null)
    if (walletAddress) {
      setBusy(`asset:${asset}`)
      setError(null)
      void refreshBootstrap(asset)
        .catch((refreshError) => {
          setError(refreshError instanceof Error ? refreshError.message : 'Failed to refresh store')
        })
        .finally(() => {
          setBusy(null)
        })
    } else {
      setSelectedAsset(asset)
    }
  }

  return (
    <main className="mx-auto flex w-full min-w-0 max-w-[1180px] flex-col gap-5 overflow-x-hidden px-4 py-5 sm:px-6 lg:px-8">
      <motion.header
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.26, ease: 'easeOut' }}
        className="glass-panel grid gap-5 rounded-lg p-5 sm:grid-cols-[1fr_auto] sm:p-6"
      >
        <div className="min-w-0">
          <p className="label-text">Content store</p>
          <h1 className="mt-2 break-words text-2xl font-black leading-tight tracking-normal text-ink sm:text-4xl">Nitrolite App Session Store</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-black/70 sm:text-base">
            YUSD-first content purchases with Yellow as a second testnet asset lane.
          </p>
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <span className="status-pill">
              <ShieldCheck className="size-3.5" />
              Sepolia
            </span>
            <span className="status-pill">
              <CheckCircle2 className="size-3.5 text-emerald-700" />
              {bootstrap?.session.status ?? 'offline'}
            </span>
          </div>
        </div>

        <div className="flex min-w-[260px] flex-col gap-3">
          <ActionButton
            className="w-full"
            onClick={connectWallet}
            disabled={busy !== null}
            icon={busy === 'connect' ? <Loader2 className="size-4 animate-spin" /> : <Wallet className="size-4" />}
          >
            {busy === 'connect' ? 'Connecting' : walletAddress ? 'Reconnect' : 'Connect'}
          </ActionButton>
          <div className="rounded-lg border border-black/10 bg-white/90 p-4 shadow-sm">
            <p className="label-text">Wallet</p>
            <p className="mt-1 truncate text-lg font-black text-ink">{shortAddress(walletAddress)}</p>
          </div>
        </div>
      </motion.header>

      <AnimatePresence>
        {error ? (
          <motion.div
            initial={{ opacity: 0, y: -8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            className="rounded-lg border border-black bg-ink px-4 py-3 text-sm font-semibold text-yellow-surface shadow-card"
            role="alert"
            aria-live="assertive"
          >
            {error}
          </motion.div>
        ) : null}
      </AnimatePresence>

      <div className="grid gap-5 lg:grid-cols-[1.05fr_0.95fr]">
        <MagicPanel>
          <PanelHeader label="Session" title={bootstrap?.store_name ?? 'Store'} icon={<CreditCard className="size-5" />} />

          <div className="mb-5 grid grid-cols-1 gap-2 rounded-lg border border-black/10 bg-black/[0.03] p-1 sm:grid-cols-2">
            {assetOptions.map((asset) => (
              <button
                key={asset}
                className={cn(
                  'min-h-10 rounded-md px-3 text-sm font-black uppercase transition',
                  selectedAsset === asset ? 'bg-ink text-yellow-brand shadow-sm' : 'text-black/60 hover:bg-white/80 hover:text-ink',
                )}
                onClick={() => switchAsset(asset)}
                disabled={!walletAddress || busy !== null}
              >
                {asset}
              </button>
            ))}
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <div className="metric-card">
              <p className="label-text">Available</p>
              <NumberTicker value={bootstrap?.available_balance ?? '0'} />
              <p className="mt-1 text-xs font-bold uppercase text-black/50">{selectedAsset}</p>
            </div>
            <div className="metric-card">
              <p className="label-text">Store balance</p>
              <NumberTicker value={bootstrap?.session.user_allocation ?? '0'} />
              <p className="mt-1 text-xs font-bold uppercase text-black/50">{selectedAsset}</p>
            </div>
            <div className="metric-card">
              <p className="label-text">App signer</p>
              <strong className="mt-1 block truncate text-lg font-black text-ink">{shortAddress(bootstrap?.app_signer ?? null)}</strong>
              <p className="mt-1 text-xs font-bold uppercase text-black/50">Quorum 2</p>
            </div>
          </div>

          <p className="mt-4 rounded-lg border border-black/10 bg-yellow-surface/70 px-3 py-2 text-sm font-semibold text-black/70" aria-live="polite">
            {bootstrap ? parseSessionDataLabel(bootstrap.session.session_data) : 'Connect wallet to load the store.'}
          </p>

          <div className="mt-5 grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto]">
            <label className="grid gap-2 text-sm font-black text-ink">
              Deposit
              <input
                id="deposit-amount"
                name="deposit_amount"
                className="min-h-11 rounded-md border border-black/10 bg-white px-3 text-base font-bold text-ink shadow-sm"
                value={depositAmount}
                onChange={(event) => setDepositAmount(event.target.value)}
                type="number"
                inputMode="decimal"
                min="0.000001"
                step="0.01"
                required
                aria-label={`Deposit amount in ${selectedAsset.toUpperCase()}`}
              />
            </label>
            <ActionButton
              className="self-end"
              disabled={!canDeposit}
              onClick={submitDeposit}
              icon={busy === 'deposit' ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            >
              {busy === 'deposit' ? 'Depositing' : 'Deposit'}
            </ActionButton>

            <label className="grid gap-2 text-sm font-black text-ink">
              Withdraw
              <input
                id="withdraw-amount"
                name="withdraw_amount"
                className="min-h-11 rounded-md border border-black/10 bg-white px-3 text-base font-bold text-ink shadow-sm"
                value={withdrawAmount}
                onChange={(event) => setWithdrawAmount(event.target.value)}
                type="number"
                inputMode="decimal"
                min="0.000001"
                step="0.01"
                required
                aria-label={`Withdraw amount in ${selectedAsset.toUpperCase()}`}
              />
            </label>
            <ActionButton
              className="self-end"
              variant="secondary"
              disabled={!canWithdraw}
              onClick={submitWithdraw}
              icon={busy === 'withdraw' ? <Loader2 className="size-4 animate-spin" /> : <CreditCard className="size-4" />}
            >
              {busy === 'withdraw' ? 'Withdrawing' : 'Withdraw'}
            </ActionButton>
          </div>
        </MagicPanel>

        <MagicPanel>
          <PanelHeader label="Library" title="What you own" icon={<Library className="size-5" />} />

          {bootstrap?.library.length ? (
            <motion.div className="grid gap-3">
              {bootstrap.library.map((item, index) => (
                <motion.div
                  key={item.id}
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: index * 0.035 }}
                  className="rounded-lg border border-black/10 bg-white/90 p-4 shadow-sm"
                >
                  <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <p className="truncate text-base font-black text-ink">{item.title}</p>
                      <p className="mt-1 line-clamp-2 text-sm leading-5 text-black/60">{item.description}</p>
                      <p className="mt-2 text-xs font-black uppercase tracking-wide text-black/50">
                        {item.type} / {item.price} {selectedAsset.toUpperCase()}
                      </p>
                    </div>
                    <ActionButton
                      variant="secondary"
                      disabled={busy !== null}
                      onClick={() => openContent(item.id)}
                      icon={busy === `content:${item.id}` ? <Loader2 className="size-4 animate-spin" /> : <BookOpen className="size-4" />}
                    >
                      {busy === `content:${item.id}` ? 'Opening' : 'Read'}
                    </ActionButton>
                  </div>
                </motion.div>
              ))}
            </motion.div>
          ) : (
            <div className="rounded-lg border border-dashed border-black/20 bg-white/60 p-5 text-sm font-semibold text-black/50">
              Your purchased items will appear here.
            </div>
          )}
        </MagicPanel>
      </div>

      <div className="grid gap-5 lg:grid-cols-[1.15fr_0.85fr]">
        <MagicPanel>
          <PanelHeader label="Catalog" title="Browse content" icon={<ShoppingBag className="size-5" />} />

          {bootstrap?.catalog.length ? (
            <div className="grid gap-3 sm:grid-cols-2">
              {bootstrap.catalog.map((item, index) => {
                const owned = libraryIds.has(item.id)
                return (
                  <motion.article
                    key={item.id}
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ delay: index * 0.035 }}
                    className="rounded-lg border border-black/10 bg-white/90 p-4 shadow-sm transition hover:-translate-y-0.5 hover:border-yellow-line hover:shadow-card"
                  >
                    <div className="flex min-h-full flex-col gap-4">
                      <div className="min-w-0">
                        <div className="mb-3 flex items-center justify-between gap-3">
                          <span className="rounded-md bg-yellow-brand px-2 py-1 text-xs font-black uppercase text-ink">Item {item.id}</span>
                          <span className="text-sm font-black text-ink">{item.prices[selectedAsset]} {selectedAsset.toUpperCase()}</span>
                        </div>
                        <h3 className="text-lg font-black tracking-normal text-ink">{item.title}</h3>
                        <p className="mt-2 line-clamp-3 text-sm leading-6 text-black/60">{item.description}</p>
                        <p className="mt-3 text-xs font-black uppercase tracking-wide text-black/40">{item.type}</p>
                      </div>
                      <ActionButton
                        className="mt-auto w-full"
                        disabled={!walletAddress || owned || busy !== null}
                        onClick={() => submitPurchase(item)}
                        icon={owned ? <CheckCircle2 className="size-4" /> : busy === `purchase:${item.id}` ? <Loader2 className="size-4 animate-spin" /> : <ShoppingBag className="size-4" />}
                      >
                        {owned ? 'Owned' : busy === `purchase:${item.id}` ? 'Purchasing' : 'Purchase'}
                      </ActionButton>
                    </div>
                  </motion.article>
                )
              })}
            </div>
          ) : (
            <div className="rounded-lg border border-dashed border-black/20 bg-white/60 p-5 text-sm font-semibold text-black/50">
              Catalog loads after wallet connection.
            </div>
          )}
        </MagicPanel>

        <MagicPanel>
          <PanelHeader label="Reader" title={readerItem?.title ?? 'Open content'} icon={<BookOpen className="size-5" />} />

          {readerItem ? (
            <article className="rounded-lg border border-black/10 bg-white/90 p-4 shadow-sm">
              <p className="mb-3 text-xs font-black uppercase tracking-wide text-black/50">
                {readerItem.type} / {readerItem.prices[selectedAsset]} {selectedAsset.toUpperCase()}
              </p>
              <pre className="whitespace-pre-wrap text-sm leading-7 text-black/75">{readerItem.content}</pre>
            </article>
          ) : (
            <div className="rounded-lg border border-dashed border-black/20 bg-white/60 p-5 text-sm font-semibold text-black/50">
              Select a library item to read.
            </div>
          )}
        </MagicPanel>
      </div>

      <MagicPanel>
        <div className="mb-5 flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
          <div>
            <p className="label-text">Browser activity</p>
            <h2 className="mt-1 text-xl font-black text-ink sm:text-2xl">Recent actions</h2>
          </div>
          <ActionButton
            variant="secondary"
            onClick={copyActivity}
            disabled={activity.length === 0}
            icon={<Copy className="size-4" />}
          >
            Copy log
          </ActionButton>
        </div>

        {activity.length ? (
          <motion.ul className="grid gap-2">
            {activity.map((entry) => (
              <motion.li
                key={entry}
                initial={{ opacity: 0, x: -6 }}
                animate={{ opacity: 1, x: 0 }}
                className="flex items-start gap-2 rounded-lg border border-black/10 bg-white/80 px-3 py-2 font-mono text-xs text-black/70"
              >
                <Activity className="mt-0.5 size-3.5 shrink-0 text-yellow-line" />
                <span className="min-w-0 overflow-hidden text-ellipsis">{entry}</span>
              </motion.li>
            ))}
          </motion.ul>
        ) : (
          <div className="rounded-lg border border-dashed border-black/20 bg-white/60 p-4 text-sm font-semibold text-black/50">
            No activity yet.
          </div>
        )}
      </MagicPanel>
    </main>
  )
}
