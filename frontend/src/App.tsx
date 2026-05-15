import { type ReactNode, useEffect, useMemo, useState } from 'react'
import Decimal from 'decimal.js'
import { AnimatePresence, motion, type HTMLMotionProps } from 'motion/react'
import { Activity, AlertTriangle, BookOpen, CheckCircle2, Copy, CreditCard, Library, Loader2, RefreshCw, ShieldCheck, ShoppingBag, Wallet } from 'lucide-react'
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
const STORED_ASSET_KEY = 'nitrolite-store:selected-asset'
const DEPOSIT_SUBMIT_TIMEOUT_MS = 45_000
const MAX_APPROVE_AMOUNT = new Decimal('1e18')
const DEFAULT_WS_URL = import.meta.env.VITE_CLEARNODE_WS_URL || 'wss://nitronode-stress.yellow.org/v1/ws'
const DEFAULT_BLOCKCHAIN_RPCS: Record<number, string> = {
  11155111: import.meta.env.VITE_BLOCKCHAIN_RPC_11155111 || 'https://ethereum-sepolia-rpc.publicnode.com',
}

type InjectedProvider = {
  request: (args: { method: string; params?: unknown[] }) => Promise<unknown>
  on?: (event: 'accountsChanged' | 'chainChanged', listener: (value?: unknown) => void) => void
  removeListener?: (event: 'accountsChanged' | 'chainChanged', listener: (value?: unknown) => void) => void
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

type RPCAppStateUpdate = {
  app_session_id: string
  intent: number
  version: string
  allocations: {
    participant: string
    asset: string
    amount: string
  }[]
  session_data: string
}

type StorePendingAction = {
  type: string
  status: string
  asset: string
  app_session_id: string
  version: number
  amount: string
  app_state_update: RPCAppStateUpdate
  user_signature: Hex
  app_signature: Hex
  created_at: string
  updated_at: string
}

type ChannelReadinessStatus = 'ready' | 'ack_required' | 'deposit_required' | 'funds_required' | 'unavailable'

type StoreChannelReadiness = {
  status: ChannelReadinessStatus
  message: string
  home_blockchain_id: number
  bootstrap_amount: string
  available_balance: string
  pending_balance: string
  requires_channel_creation: boolean
  pending_transition?: string
  pending_amount?: string
  on_chain_balance: string
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
  channel_readiness: StoreChannelReadiness
  catalog: StoreCatalogItem[]
  session: StoreSession
  library: StoreLibraryItem[]
  pending_action?: StorePendingAction
}

type ContentResponse = StoreCatalogItem

type StoreUpdateResponse = {
  status: string
  intent: string
  asset: string
  app_session_id: string
  app_signature?: Hex
  pending_action?: StorePendingAction
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

function loadStoredAsset(): string {
  try {
    const stored = window.localStorage.getItem(STORED_ASSET_KEY)
    return stored && DEMO_ASSETS.includes(stored) ? stored : DEFAULT_ASSET
  } catch {
    return DEFAULT_ASSET
  }
}

function persistAsset(asset: string) {
  try {
    window.localStorage.setItem(STORED_ASSET_KEY, asset)
  } catch {
    // Local storage is best-effort; wallet recovery still works without it.
  }
}

async function createWalletConnections(provider: InjectedProvider, wallet: string) {
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

  return { client, nitrolite }
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

function parseDecimal(value: string): Decimal | null {
  try {
    const amount = new Decimal(value)
    return amount.isFinite() ? amount : null
  } catch {
    return null
  }
}

function isPositiveAmount(value: string): boolean {
  return parseDecimal(value)?.greaterThan(0) ?? false
}

function minDecimal(a: Decimal, b: Decimal): Decimal {
  return a.lessThan(b) ? a : b
}

function isAllowanceError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error)
  const normalized = message.toLowerCase()
  return normalized.includes('allowance') && normalized.includes('sufficient')
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

function channelReadinessLabel(status?: ChannelReadinessStatus) {
  switch (status) {
    case 'ready':
      return 'ready'
    case 'ack_required':
      return 'ack required'
    case 'deposit_required':
      return 'deposit required'
    case 'funds_required':
      return 'funds needed'
    case 'unavailable':
      return 'sync unavailable'
    default:
      return 'offline'
  }
}

function channelReadinessMessage(readiness: StoreChannelReadiness | undefined, asset: string) {
  switch (readiness?.status) {
    case 'ready':
      return `Home channel is ready with ${formatAmount(readiness.available_balance)} ${asset.toUpperCase()}.`
    case 'ack_required':
      if (readiness.requires_channel_creation) {
        return `${formatAmount(readiness.pending_balance)} ${asset.toUpperCase()} was received off-chain. Sign once to open a home channel and make it available.`
      }
      return `Acknowledge ${formatAmount(readiness.pending_balance)} ${asset.toUpperCase()} in your pending channel state.`
    case 'deposit_required':
      return `Prepare a home channel with up to ${formatAmount(readiness.bootstrap_amount)} ${asset.toUpperCase()} from your wallet.`
    case 'funds_required':
      return `Add ${asset.toUpperCase()} test funds to this wallet before preparing a home channel.`
    case 'unavailable':
      return readiness.message || 'Channel readiness could not be checked.'
    default:
      return 'Connect wallet to check channel readiness.'
  }
}

function sessionStatusLabel(status?: string) {
  switch (status) {
    case 'open':
      return 'ready'
    case 'missing':
      return 'setup required'
    case 'sync_failed':
      return 'sync pending'
    case 'closed':
      return 'closed'
    default:
      return status ?? 'offline'
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

function fromRPCAppStateUpdate(update: RPCAppStateUpdate): AppStateUpdateV1 {
  return {
    appSessionId: update.app_session_id,
    intent: Number(update.intent) as AppStateUpdateIntent,
    version: BigInt(update.version),
    allocations: update.allocations.map((allocation) => ({
      participant: allocation.participant as Address,
      asset: allocation.asset,
      amount: new Decimal(allocation.amount),
    })),
    sessionData: update.session_data,
  }
}

async function withTimeout<T>(promise: Promise<T>, timeoutMs: number, message: string): Promise<T> {
  let timeoutID: number | undefined
  const timeout = new Promise<never>((_, reject) => {
    timeoutID = window.setTimeout(() => reject(new Error(message)), timeoutMs)
  })
  try {
    return await Promise.race([promise, timeout])
  } finally {
    if (timeoutID !== undefined) window.clearTimeout(timeoutID)
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
  const [selectedAsset, setSelectedAsset] = useState(loadStoredAsset)
  const [bootstrap, setBootstrap] = useState<StoreBootstrap | null>(null)
  const [depositAmount, setDepositAmount] = useState('1.00')
  const [withdrawAmount, setWithdrawAmount] = useState('0.50')
  const [readerItem, setReaderItem] = useState<ContentResponse | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [activity, setActivity] = useState<string[]>([])

  const libraryIds = useMemo(() => new Set((bootstrap?.library ?? []).map((item) => item.id)), [bootstrap])
  const assetOptions = useMemo(() => {
    const supported = new Set(bootstrap?.supported_assets ?? DEMO_ASSETS)
    return DEMO_ASSETS.filter((asset) => supported.has(asset))
  }, [bootstrap])
  const sessionReady = Boolean(bootstrap?.session.app_session_id && bootstrap.session.status === 'open')
  const sessionNeedsStart = bootstrap?.session.status === 'missing' || bootstrap?.session.status === 'sync_failed'
  const pendingDeposit = bootstrap?.pending_action?.type === 'user_deposit' ? bootstrap.pending_action : null
  const channelReadiness = bootstrap?.channel_readiness
  const channelReady = channelReadiness?.status === 'ready'
  const canRunChannelSetup = channelReadiness?.status === 'ack_required' || channelReadiness?.status === 'deposit_required'
  const availableBalance = useMemo(() => parseDecimal(bootstrap?.available_balance ?? '0') ?? new Decimal(0), [bootstrap?.available_balance])
  const pendingChannelBalance = useMemo(
    () => parseDecimal(channelReadiness?.pending_balance ?? bootstrap?.available_balance ?? '0') ?? new Decimal(0),
    [bootstrap?.available_balance, channelReadiness?.pending_balance],
  )
  const pendingChannelDelta = useMemo(() => {
    const delta = pendingChannelBalance.minus(availableBalance)
    return delta.greaterThan(0) ? delta : new Decimal(0)
  }, [availableBalance, pendingChannelBalance])
  const pendingChannelAmount = useMemo(
    () => parseDecimal(channelReadiness?.pending_amount ?? pendingChannelDelta.toFixed()) ?? pendingChannelDelta,
    [channelReadiness?.pending_amount, pendingChannelDelta],
  )
  const hasPendingChannelBalance = Boolean(channelReadiness?.status === 'ack_required' && pendingChannelBalance.greaterThan(availableBalance))
  const requiresChannelCreation = Boolean(channelReadiness?.requires_channel_creation)
  const hasWithdrawnChannelBalance = Boolean(
    channelReadiness?.status === 'ack_required' && channelReadiness.pending_transition === 'release' && pendingChannelAmount.greaterThan(0),
  )
  const depositValue = useMemo(() => parseDecimal(depositAmount), [depositAmount])
  const pendingDepositValue = useMemo(() => (pendingDeposit ? parseDecimal(pendingDeposit.amount) : null), [pendingDeposit])
  const depositExceedsAvailable = Boolean(bootstrap && depositValue?.greaterThan(0) && depositValue.greaterThan(availableBalance))
  const pendingDepositExceedsAvailable = Boolean(bootstrap && pendingDepositValue?.greaterThan(0) && pendingDepositValue.greaterThan(availableBalance))
  const canPrepareChannel = Boolean(walletAddress && nitroliteClient && bootstrap && busy === null && canRunChannelSetup)
  const canDeposit = Boolean(walletAddress && sessionReady && channelReady && busy === null && isPositiveAmount(depositAmount) && !depositExceedsAvailable)
  const canWithdraw = Boolean(walletAddress && sessionReady && channelReady && busy === null && isPositiveAmount(withdrawAmount))
  const canResumeDeposit = Boolean(walletAddress && nitroliteClient && pendingDeposit && busy === null && !pendingDepositExceedsAvailable)
  const canCreateSession = Boolean(sessionNeedsStart && walletAddress && walletClient && busy === null && channelReady)

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

  async function hydrateWallet(provider: InjectedProvider, wallet: string, asset: string, label: string) {
    await ensureSepolia(provider)
    const { client, nitrolite } = await createWalletConnections(provider, wallet)
    setWalletAddress(wallet)
    setWalletClient(client)
    setNitroliteClient(nitrolite)
    appendLog(`${label} ${shortAddress(wallet)}`)
    await refreshBootstrap(asset, wallet)
  }

  useEffect(() => {
    const provider = getMetaMaskProvider()
    if (!provider) return

    let cancelled = false
    const restore = async (wallet: string) => {
      setBusy('restore')
      setError(null)
      try {
        await hydrateWallet(provider, wallet, loadStoredAsset(), 'wallet restored')
      } catch (restoreError) {
        if (!cancelled) {
          setError(restoreError instanceof Error ? restoreError.message : 'Failed to restore wallet')
        }
      } finally {
        if (!cancelled) setBusy(null)
      }
    }

    void provider
      .request({ method: 'eth_accounts' })
      .then((accounts) => {
        if (cancelled || !Array.isArray(accounts) || typeof accounts[0] !== 'string') return
        return restore(accounts[0])
      })
      .catch(() => undefined)

    const onAccountsChanged = (value?: unknown) => {
      const wallet = Array.isArray(value) && typeof value[0] === 'string' ? value[0] : null
      if (!wallet) {
        setWalletAddress(null)
        setWalletClient(null)
        setNitroliteClient(null)
        setBootstrap(null)
        setReaderItem(null)
        appendLog('wallet disconnected')
        return
      }
      void restore(wallet)
    }
    const onChainChanged = () => {
      void provider
        .request({ method: 'eth_accounts' })
        .then((accounts) => {
          if (!Array.isArray(accounts) || typeof accounts[0] !== 'string') return
          return restore(accounts[0])
        })
        .catch(() => undefined)
    }

    provider.on?.('accountsChanged', onAccountsChanged)
    provider.on?.('chainChanged', onChainChanged)
    return () => {
      cancelled = true
      provider.removeListener?.('accountsChanged', onAccountsChanged)
      provider.removeListener?.('chainChanged', onChainChanged)
    }
    // Run once so refreshes do not repeatedly recreate the SDK client.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function refreshBootstrap(asset: string, walletOverride?: string | null) {
    const wallet = walletOverride ?? walletAddress
    if (!wallet) {
      throw new Error('Connect a wallet before bootstrapping the store.')
    }
    const next = await readJSON<StoreBootstrap>(
      `/api/store/bootstrap?asset=${encodeURIComponent(asset)}&wallet_address=${encodeURIComponent(wallet)}`,
    )
    setBootstrap(next)
    setSelectedAsset(next.selected_asset)
    persistAsset(next.selected_asset)
    appendLog(`bootstrap ${asset.toUpperCase()} -> ${sessionStatusLabel(next.session.status)}`)
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

      await hydrateWallet(provider, wallet, selectedAsset, 'wallet connected')
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

  async function startSession() {
    if (!bootstrap || !walletClient || !walletAddress) return
    if (!sessionNeedsStart) return

    setBusy('create-session')
    setError(null)
    try {
      const created = await createSession(bootstrap, walletClient, walletAddress)
      setBootstrap(created)
      appendLog(`session created ${shortAddress(created.session.app_session_id ?? '')}`)
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : 'Failed to create store session')
    } finally {
      setBusy(null)
    }
  }

  async function checkpointWithApproval(asset: string): Promise<string | null> {
    if (!nitroliteClient || !walletAddress || !bootstrap) throw new Error('Connect a wallet before preparing a channel.')
    try {
      return await nitroliteClient.checkpoint(asset)
    } catch (checkpointError) {
      if (!isAllowanceError(checkpointError)) {
        const message = checkpointError instanceof Error ? checkpointError.message : String(checkpointError)
        if (message.toLowerCase().includes('does not require a blockchain operation')) return null
        throw checkpointError
      }
      const signedState = await nitroliteClient.getLatestState(walletAddress as Address, asset, true)
      const chainID = signedState.homeLedger.blockchainId || BigInt(bootstrap.channel_readiness.home_blockchain_id)
      appendLog(`approve ${asset.toUpperCase()} channel spend`)
      await nitroliteClient.approveToken(chainID, asset, MAX_APPROVE_AMOUNT)
      return await nitroliteClient.checkpoint(asset)
    }
  }

  async function prepareChannel() {
    if (!bootstrap || !walletAddress || !nitroliteClient) return
    const readiness = bootstrap.channel_readiness
    const asset = bootstrap.selected_asset
    if (readiness.status !== 'ack_required' && readiness.status !== 'deposit_required') return

    setBusy('prepare-channel')
    setError(null)
    try {
      if (readiness.status === 'ack_required') {
        appendLog(`acknowledge ${asset.toUpperCase()} channel state`)
        try {
          await nitroliteClient.acknowledge(asset)
        } catch (ackError) {
          const message = ackError instanceof Error ? ackError.message : String(ackError)
          if (!message.toLowerCase().includes('already acknowledged')) throw ackError
        }
      } else {
        const chainID = BigInt(readiness.home_blockchain_id)
        const configuredAmount = parseDecimal(readiness.bootstrap_amount) ?? new Decimal(10)
        const onChainBalance = await nitroliteClient.getOnChainBalance(chainID, asset, walletAddress as Address)
        const amount = minDecimal(configuredAmount, onChainBalance)
        if (!amount.greaterThan(0)) {
          throw new Error(`Add ${asset.toUpperCase()} test funds to this wallet before preparing a home channel.`)
        }
        appendLog(`prepare ${asset.toUpperCase()} channel ${amount.toFixed()}`)
        try {
          await nitroliteClient.deposit(chainID, asset, amount)
        } catch (depositError) {
          if (!isAllowanceError(depositError)) throw depositError
          appendLog(`approve ${asset.toUpperCase()} channel spend`)
          await nitroliteClient.approveToken(chainID, asset, MAX_APPROVE_AMOUNT)
          await nitroliteClient.deposit(chainID, asset, amount)
        }
      }

      const txHash = await checkpointWithApproval(asset)
      appendLog(txHash ? `channel checkpoint ${shortAddress(txHash)}` : `channel state synced`)
      await refreshBootstrap(asset)
    } catch (setupError) {
      setError(setupError instanceof Error ? setupError.message : 'Failed to prepare channel')
      try {
        await refreshBootstrap(asset)
      } catch {
        // Keep the setup error visible.
      }
    } finally {
      setBusy(null)
    }
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
      if (!amount.greaterThan(0)) {
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
      const message = withdrawError instanceof Error ? withdrawError.message : 'Failed to withdraw'
      setError(
        message.startsWith('failed to submit app state: ')
          ? `Nitronode rejected the withdraw state: ${message.slice('failed to submit app state: '.length)}`
          : message,
      )
      try {
        await refreshBootstrap(bootstrap.selected_asset)
      } catch {
        // Keep the withdraw error visible.
      }
    } finally {
      setBusy(null)
    }
  }

  async function finishDeposit(appStateUpdate: AppStateUpdateV1, userSignature: Hex, appSignature: Hex, asset: string, amount: Decimal) {
    if (!nitroliteClient) throw new Error('Connect a wallet before submitting the deposit.')
    appendLog(`deposit signed ${amount.toFixed()} ${asset.toUpperCase()}; submitting`)
    await assertHomeChannelCanDeposit(asset, amount)
    await withTimeout(
      nitroliteClient.submitAppSessionDeposit(appStateUpdate, [userSignature, appSignature], asset, amount),
      DEPOSIT_SUBMIT_TIMEOUT_MS,
      'Deposit is signed, but the Nitronode submit did not finish yet. Use Resume deposit after refresh.',
    )
    await refreshBootstrap(asset)
    appendLog(`deposit ${amount.toFixed()} ${asset.toUpperCase()} submitted`)
  }

  async function assertHomeChannelCanDeposit(asset: string, amount: Decimal) {
    if (!nitroliteClient || !walletAddress) throw new Error('Connect a wallet before submitting the deposit.')
    let state
    try {
      state = await nitroliteClient.getLatestState(walletAddress as Address, asset, true)
    } catch {
      throw new Error(`Open and fund a ${asset.toUpperCase()} home channel before using this store.`)
    }
    const channelBalance = new Decimal(state.homeLedger.userBalance)
    if (channelBalance.lessThan(amount)) {
      throw new Error(`Deposit amount exceeds your available ${asset.toUpperCase()} channel funds.`)
    }
  }

  async function resumeDeposit() {
    if (!pendingDeposit) return

    setBusy('resume-deposit')
    setError(null)
    try {
      const amount = new Decimal(pendingDeposit.amount)
      const appStateUpdate = fromRPCAppStateUpdate(pendingDeposit.app_state_update)
      await finishDeposit(appStateUpdate, pendingDeposit.user_signature, pendingDeposit.app_signature, pendingDeposit.asset, amount)
    } catch (resumeError) {
      setError(resumeError instanceof Error ? resumeError.message : 'Failed to resume deposit')
      try {
        await refreshBootstrap(pendingDeposit.asset)
      } catch {
        // The original recovery error is more useful to the user.
      }
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
      if (!amount.greaterThan(0)) {
        throw new Error('Deposit amount must be greater than zero.')
      }
      const available = new Decimal(bootstrap.available_balance || '0')
      if (amount.greaterThan(available)) {
        if (!available.greaterThan(0)) {
          throw new Error(`Open and fund a ${bootstrap.selected_asset.toUpperCase()} home channel before depositing.`)
        }
        throw new Error(`Deposit amount exceeds your available ${bootstrap.selected_asset.toUpperCase()} channel funds.`)
      }
      const version = bootstrap.session.version + 1
      const currentUser = new Decimal(bootstrap.session.user_allocation)
      const currentApp = new Decimal(bootstrap.session.app_allocation)
      const nextUser = currentUser.plus(amount)

      const sessionData = JSON.stringify({ intent: 'user_deposit', amount: amount.toFixed() })
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
      await finishDeposit(appStateUpdate, userSignature, result.app_signature, bootstrap.selected_asset, amount)
    } catch (depositError) {
      setError(depositError instanceof Error ? depositError.message : 'Failed to add funds')
      try {
        await refreshBootstrap(bootstrap.selected_asset)
      } catch {
        // Preserve the deposit error while leaving any checkpoint visible if bootstrap succeeds.
      }
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
    persistAsset(asset)
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

  function channelSetupActionLabel() {
    if (busy === 'prepare-channel') return 'Preparing'
    switch (channelReadiness?.status) {
      case 'ack_required':
        return 'Make available'
      case 'deposit_required':
        return 'Prepare channel'
      case 'funds_required':
        return 'Funds needed'
      case 'unavailable':
        return 'Reconnect'
      default:
        return 'Prepare channel'
    }
  }

  function renderChannelSetupPanel() {
    if (!bootstrap || channelReady) return null
    const title = hasWithdrawnChannelBalance
      ? 'Make withdrawn balance available'
      : requiresChannelCreation
        ? 'Make received funds available'
      : channelReadiness?.status === 'ack_required'
        ? 'Make pending balance available'
        : channelReadinessLabel(channelReadiness?.status)
    const message = hasWithdrawnChannelBalance
      ? `${formatAmount(pendingChannelAmount.toFixed())} ${selectedAsset.toUpperCase()} from your store withdrawal is pending. Sign once to add it back to your available channel balance.`
      : requiresChannelCreation
        ? `${formatAmount(pendingChannelBalance.toFixed())} ${selectedAsset.toUpperCase()} was received off-chain. Sign once, then checkpoint to open your home channel and make it available.`
      : channelReadiness?.status === 'ack_required'
        ? `${formatAmount(pendingChannelBalance.toFixed())} ${selectedAsset.toUpperCase()} is pending in your channel. Sign once to make it available.`
        : channelReadinessMessage(channelReadiness, selectedAsset)

    return (
      <div className="mt-4 flex flex-col gap-3 rounded-lg border border-black/10 bg-white/90 p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <p className="flex items-center gap-2 text-sm font-black uppercase tracking-normal text-ink">
            <ShieldCheck className="size-4 shrink-0" />
            {title}
          </p>
          <p className="mt-1 text-sm font-semibold leading-5 text-black/60">
            {message}
          </p>
          {hasPendingChannelBalance ? (
            <div className="mt-3 grid gap-2 text-xs font-black text-black/60 sm:grid-cols-2">
              <span className="rounded-md border border-black/10 bg-white px-3 py-2">
                Available now: {formatAmount(availableBalance.toFixed())} {selectedAsset.toUpperCase()}
              </span>
              <span className="rounded-md border border-yellow-line bg-yellow-brand/20 px-3 py-2 text-ink">
                After signing: {formatAmount(pendingChannelBalance.toFixed())} {selectedAsset.toUpperCase()}
              </span>
            </div>
          ) : null}
        </div>
        <ActionButton
          className="shrink-0"
          disabled={!canPrepareChannel}
          onClick={prepareChannel}
          icon={busy === 'prepare-channel' ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
        >
          {channelSetupActionLabel()}
        </ActionButton>
      </div>
    )
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
              {sessionStatusLabel(bootstrap?.session.status)}
            </span>
          </div>
        </div>

        <div className="flex min-w-[260px] flex-col gap-3">
          <ActionButton
            className="w-full"
            onClick={connectWallet}
            disabled={busy !== null}
            icon={busy === 'connect' || busy === 'restore' ? <Loader2 className="size-4 animate-spin" /> : <Wallet className="size-4" />}
          >
            {busy === 'restore' ? 'Restoring' : busy === 'connect' ? 'Connecting' : walletAddress ? 'Reconnect' : 'Connect'}
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
              {hasPendingChannelBalance ? (
                <p className="mt-1 text-xs font-bold text-amber-700">
                  Pending: {formatAmount(pendingChannelBalance.toFixed())} {selectedAsset.toUpperCase()}
                </p>
              ) : null}
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

          {sessionNeedsStart && channelReady ? (
            <div className="mt-4 flex flex-col gap-3 rounded-lg border border-black/10 bg-white/90 p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm font-semibold leading-5 text-black/60">
                {bootstrap?.session.status === 'sync_failed'
                  ? 'The previous store session is not synced. Sign once to start a fresh store session.'
                  : 'Sign once to start a store session and unlock deposits.'}
              </p>
              <ActionButton
                className="shrink-0"
                disabled={!canCreateSession}
                onClick={startSession}
                icon={busy === 'create-session' ? <Loader2 className="size-4 animate-spin" /> : <CreditCard className="size-4" />}
              >
                {busy === 'create-session' ? 'Starting' : 'Sign to start'}
              </ActionButton>
            </div>
          ) : null}

          {sessionNeedsStart ? renderChannelSetupPanel() : null}

          {pendingDeposit ? (
            <div className="mt-4 flex flex-col gap-3 rounded-lg border border-yellow-line bg-yellow-brand/25 p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <p className="flex items-center gap-2 text-sm font-black text-ink">
                  <AlertTriangle className="size-4 shrink-0" />
                  Deposit checkpoint ready
                </p>
                <p className="mt-1 text-sm font-semibold leading-5 text-black/60">
                  {pendingDepositExceedsAvailable
                    ? `${formatAmount(pendingDeposit.amount)} ${pendingDeposit.asset.toUpperCase()} exceeds the current available balance. Enter a smaller deposit to replace it, or top up before resuming.`
                    : `${formatAmount(pendingDeposit.amount)} ${pendingDeposit.asset.toUpperCase()} is signed at version ${pendingDeposit.version}. Resume submits it to Nitronode without another MetaMask prompt.`}
                </p>
              </div>
              <ActionButton
                className="shrink-0"
                variant="secondary"
                disabled={!canResumeDeposit}
                onClick={resumeDeposit}
                icon={busy === 'resume-deposit' ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
              >
                {busy === 'resume-deposit' ? 'Resuming' : 'Resume'}
              </ActionButton>
            </div>
          ) : null}

          {sessionReady && !channelReady ? renderChannelSetupPanel() : null}

          {sessionReady && channelReady ? (
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
                {depositExceedsAvailable ? (
                  <span className="text-xs font-bold leading-5 text-red-700">
                    {`Deposit amount exceeds your available ${selectedAsset.toUpperCase()} channel funds.`}
                  </span>
                ) : pendingDeposit ? (
                  <span className="text-xs font-bold leading-5 text-black/50">A new deposit replaces the pending checkpoint.</span>
                ) : null}
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
          ) : null}
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
