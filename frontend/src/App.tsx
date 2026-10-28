import { useEffect, useState, type FormEvent } from 'react'
import './App.css'

const roles = ['PO', 'QA', 'Admin', 'Scrum Master', 'Desenvolvedor'] as const
type Role = (typeof roles)[number]

type User = {
  id: number
  username: string
  displayName: string
  role: Role
  active: boolean
  createdAt?: string
}

type Project = {
  id: number
  name: string
  description: string
  requirements: string
  workspacePath: string
  createdAt: string
}

type Task = {
  id: number
  projectId: number
  title: string
  description: string
  state: string
  specificationStatus: string
  implementationStatus: string
  qaStatus: string
  prStatus: string
  checksStatus: string
  createdAt: string
}

type TaskEvent = {
  id: number
  actorName: string
  actorRole: Role
  type: 'transition' | 'approval' | 'comment' | 'failure' | 'attempt'
  action: string
  fromState: string
  toState: string
  comment?: string
  reason?: string
  occurredAt: string
}

type TaskSpecification = {
  id: number
  taskId: number
  round: number
  status: string
  artifactPaths: string[]
  summary: string
  createdAt: string
}

type SpecificationArtifact = { path: string; content: string }
type TaskImplementation = {
  id: number
  taskId: number
  attempt: number
  status: string
  changedFiles: string[]
  summary: string
  createdAt: string
}
type QAEnvironment = { projectId: number; taskId: number; status: 'starting' | 'healthy' | 'unhealthy' | 'stopped'; url?: string; port?: number; healthCheckedAt?: string }
type TaskPullRequest = { number: number; url: string; branch: string; checks?: { name: string; state: string; link: string; workflow: string }[] }

type Page = 'loading' | 'setup' | 'login' | 'projects' | 'project' | 'task' | 'users' | 'error'

const taskColumns = [
  { title: 'Preparação', states: ['Aberta', 'Pronta'] },
  { title: 'Especificação', states: ['Especificando', 'Aguardando aprovação das especificações', 'Ajustes nas especificações'] },
  { title: 'Implementação', states: ['Em desenvolvimento', 'Correção necessária'] },
  { title: 'QA', states: ['Aguardando QA', 'Em QA'] },
  { title: 'Entrega', states: ['Aguardando PR', 'PR aberto', 'Pronta para merge'] },
  { title: 'Encerradas', states: ['Bloqueada', 'Concluída'] },
]

function canOrganize(role?: Role) { return role === 'Admin' || role === 'PO' || role === 'Scrum Master' }
function canMerge(role?: Role) { return role === 'Admin' || role === 'PO' || role === 'QA' || role === 'Desenvolvedor' }

function normalizeSpecifications(items: TaskSpecification[]) {
  return items.map((item) => ({ ...item, artifactPaths: item.artifactPaths ?? [] }))
}

function normalizeImplementations(items: TaskImplementation[]) {
  return items.map((item) => ({ ...item, changedFiles: item.changedFiles ?? [] }))
}

function phaseStatus(value: string) {
  const labels: Record<string, string> = {
    not_started: 'Não iniciada', pending: 'Pendente', approved: 'Aprovada',
    changes_requested: 'Ajustes pedidos', running: 'Em andamento', succeeded: 'Concluída',
    failed: 'Falhou', rejected: 'Reprovada', not_created: 'Não criado', open: 'Aberto',
    merged: 'Mesclado', unknown: 'Aguardando', passed: 'Aprovados',
  }
  return labels[value] ?? value
}

function eventActionLabel(action: string) {
  const labels: Record<string, string> = {
    mark_ready: 'marcou como pronta', start_specification: 'iniciou a especificação',
    specification_generated: 'gerou a especificação', approve_specification: 'aprovou a especificação',
    specification_failed: 'registrou falha na especificação',
    return_specification: 'devolveu a especificação', implementation_started: 'iniciou a implementação',
    implementation_succeeded: 'concluiu a implementação', implementation_failed: 'registrou falha na implementação',
    qa_environment_start_failed: 'falhou ao iniciar o ambiente de QA',
    qa_started: 'iniciou o QA', approve_qa: 'aprovou o QA', reject_qa: 'reprovou o QA',
    start_correction: 'iniciou a correção', pr_created: 'abriu o pull request', checks_pending: 'registrou checks pendentes',
    checks_failed: 'registrou checks reprovados', checks_passed: 'registrou checks aprovados',
    merge: 'concluiu o merge', block: 'bloqueou a tarefa', unblock: 'liberou a tarefa',
  }
  return labels[action] ?? action.replaceAll('_', ' ')
}

function eventTypeLabel(type: TaskEvent['type']) {
  const labels: Record<TaskEvent['type'], string> = {
    transition: 'transição', approval: 'decisão', comment: 'comentário', failure: 'falha', attempt: 'tentativa negada',
  }
  return labels[type]
}

function nextTaskAction(task: Task, role?: Role) {
  switch (task.state) {
    case 'Aberta': return canOrganize(role) ? 'Prepare a descrição e marque a tarefa como pronta.' : 'PO ou Scrum Master deve preparar e marcar a tarefa como pronta.'
    case 'Pronta': return task.specificationStatus === 'failed' ? 'A geração falhou. PO ou Scrum Master pode tentar novamente.' : 'OpenCode deve gerar os artefatos de especificação para revisão do PO.'
    case 'Especificando': return 'A geração da especificação está em andamento.'
    case 'Aguardando aprovação das especificações': return role === 'PO' || role === 'Admin' ? 'Revise os artefatos e aprove ou devolva com um comentário.' : 'Aguardando revisão do PO.'
    case 'Ajustes nas especificações': return canOrganize(role) ? 'Inicie uma nova rodada de especificação após os ajustes.' : 'Aguardando nova rodada de especificação.'
    case 'Em desenvolvimento': return task.implementationStatus === 'failed' ? 'A implementação falhou. Desenvolvedor pode tentar novamente.' : 'OpenCode deve implementar os artefatos aprovados.'
    case 'Aguardando QA': return role === 'QA' || role === 'Admin' ? 'Inicie a revisão de QA para avaliar a implementação.' : 'Aguardando QA.'
    case 'Em QA': return role === 'QA' || role === 'Admin' ? 'Registre o resultado de QA; uma reprovação exige comentário.' : 'Aguardando decisão de QA.'
    case 'Correção necessária': return role === 'Desenvolvedor' || role === 'Admin' ? 'Inicie a correção antes de solicitar uma nova validação.' : 'Aguardando correção do Desenvolvedor.'
    case 'Aguardando PR': return 'A criação do pull request fica disponível após a aprovação de QA.'
    case 'PR aberto': return task.checksStatus === 'failed' ? 'Há checks reprovados. Corrija o PR e aguarde uma nova execução.' : 'Aguardando os checks obrigatórios do pull request.'
    case 'Pronta para merge': return canMerge(role) ? 'Todos os checks passaram. Você pode fazer o merge.' : 'Aguardando merge por PO, QA, Desenvolvedor ou Admin.'
    case 'Bloqueada': return role === 'Admin' ? 'Revise o motivo do bloqueio e retome o estado anterior.' : 'Aguardando o Admin liberar a tarefa.'
    case 'Concluída': return 'Tarefa concluída após o merge.'
    default: return 'Consulte o histórico para identificar a próxima ação.'
  }
}

async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...options,
    credentials: 'same-origin',
    headers: { ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...options.headers },
  })
  if (response.status === 204) return undefined as T
  const body = await response.text()
  if (!response.ok) throw new Error(body.trim() || `A solicitação falhou (${response.status}).`)
  return body ? (JSON.parse(body) as T) : (undefined as T)
}

function App() {
  const [page, setPage] = useState<Page>('loading')
  const [currentUser, setCurrentUser] = useState<User | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [selectedProject, setSelectedProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [selectedTask, setSelectedTask] = useState<Task | null>(null)
  const [events, setEvents] = useState<TaskEvent[]>([])
  const [specifications, setSpecifications] = useState<TaskSpecification[]>([])
  const [specificationArtifact, setSpecificationArtifact] = useState<SpecificationArtifact | null>(null)
  const [implementations, setImplementations] = useState<TaskImplementation[]>([])
  const [qaEnvironment, setQAEnvironment] = useState<QAEnvironment | null>(null)
  const [qaLogs, setQALogs] = useState('')
  const [taskPullRequest, setTaskPullRequest] = useState<TaskPullRequest | null>(null)
  const [pullRequestChecks, setPullRequestChecks] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [confirmingId, setConfirmingId] = useState<number | null>(null)
  const [showProjectForm, setShowProjectForm] = useState(false)
  const [showTaskForm, setShowTaskForm] = useState(false)
  const [editingTask, setEditingTask] = useState(false)
  const [actionComment, setActionComment] = useState('')
  const [qaEvidence, setQAEvidence] = useState('')

  useEffect(() => {
    let active = true
    async function initialize() {
      try {
        const setup = await api<{ needsAdminSetup: boolean }>('/api/setup/status')
        if (!active) return
        if (setup.needsAdminSetup) {
          setPage('setup')
          return
        }
        const user = await api<User>('/api/session')
        if (!active) return
        setCurrentUser(user)
        setProjects(await api<Project[]>('/api/projects'))
        if (active) setPage('projects')
      } catch (error) {
        if (!active) return
        const text = error instanceof Error ? error.message : 'Não foi possível conectar à fábrica.'
        if (text.includes('authentication required')) setPage('login')
        else {
          setMessage(text)
          setPage('error')
        }
      }
    }
    void initialize()
    return () => { active = false }
  }, [])

  useEffect(() => {
    const taskID = selectedTask?.id
    const specificationRunning = selectedTask?.state === 'Especificando'
    const implementationRunning = selectedTask?.state === 'Em desenvolvimento' && selectedTask.implementationStatus === 'running'
    if (page !== 'task' || !taskID || (!specificationRunning && !implementationRunning)) return
    let active = true
    const refresh = async () => {
      try {
        const task = await api<Task>(`/api/tasks/${taskID}`)
        if (!active) return
        setSelectedTask(task)
        setTasks((current) => current.map((item) => item.id === task.id ? task : item))
        const rounds = await api<TaskSpecification[]>(`/api/tasks/${task.id}/specifications`)
        if (!active) return
        const safeRounds = normalizeSpecifications(rounds)
        setSpecifications(safeRounds)
        setImplementations(normalizeImplementations(await api<TaskImplementation[]>(`/api/tasks/${task.id}/implementations`)))
        const latest = safeRounds.at(-1)
        if (task.state !== 'Especificando' && latest?.artifactPaths.length) {
          const artifact = await api<SpecificationArtifact>(`/api/tasks/${task.id}/specifications/${latest.round}/artifact?path=${encodeURIComponent(latest.artifactPaths[0])}`)
          if (active) setSpecificationArtifact(artifact)
        } else if (task.state !== 'Especificando') setSpecificationArtifact(null)
      } catch {
        // Keep the task view usable while the local agent run is being reconciled.
      }
    }
    const timer = window.setInterval(() => void refresh(), 1200)
    return () => { active = false; window.clearInterval(timer) }
  }, [page, selectedTask?.id, selectedTask?.state, selectedTask?.implementationStatus])

  useEffect(() => {
    const taskID = selectedTask?.id
    const qaReady = selectedTask?.state === 'Aguardando QA' || selectedTask?.state === 'Em QA'
    if (page !== 'task' || !taskID || !qaReady) return
    let active = true
    let timer: number | undefined
    const refresh = async () => {
      try {
        const environment = await api<QAEnvironment>(`/api/tasks/${taskID}/environment`)
        if (!active) return
        setQAEnvironment(environment)
        if (environment.status !== 'starting') {
          const latestEvents = await api<TaskEvent[]>(`/api/tasks/${taskID}/events`)
          if (active) setEvents(latestEvents)
          if (timer !== undefined) window.clearInterval(timer)
        }
      } catch {
        // Keep checking while the server is bringing the QA environment up.
      }
    }
    timer = window.setInterval(() => void refresh(), 1200)
    void refresh()
    return () => { active = false; window.clearInterval(timer) }
  }, [page, selectedTask?.id, selectedTask?.state])

  async function enterFactory(user: User) {
    setCurrentUser(user)
    setMessage('')
    try {
      setProjects(await api<Project[]>('/api/projects'))
      setPage('projects')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível carregar os projetos.')
      setPage('error')
    }
  }

  async function openProject(project: Project) {
    setBusy(true)
    setMessage('')
    try {
      setSelectedProject(project)
      setTasks(await api<Task[]>(`/api/projects/${project.id}/tasks`))
      setSelectedTask(null)
      setPage('project')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível abrir o projeto.')
    } finally {
      setBusy(false)
    }
  }

  async function openTask(task: Task) {
    setBusy(true)
    setMessage('')
    try {
      setSelectedTask(task)
      setQAEvidence('')
      setQAEnvironment(await api<QAEnvironment>(`/api/tasks/${task.id}/environment`))
      setQALogs('')
      const pull = await api<TaskPullRequest>(`/api/tasks/${task.id}/pull-request`)
      setTaskPullRequest(pull.number ? pull : null)
      setPullRequestChecks('')
      setEvents(await api<TaskEvent[]>(`/api/tasks/${task.id}/events`))
      setImplementations(normalizeImplementations(await api<TaskImplementation[]>(`/api/tasks/${task.id}/implementations`)))
      const rounds = normalizeSpecifications(await api<TaskSpecification[]>(`/api/tasks/${task.id}/specifications`))
      setSpecifications(rounds)
      const latest = rounds.at(-1)
      if (latest?.artifactPaths.length) {
        setSpecificationArtifact(await api<SpecificationArtifact>(`/api/tasks/${task.id}/specifications/${latest.round}/artifact?path=${encodeURIComponent(latest.artifactPaths[0])}`))
      } else setSpecificationArtifact(null)
      setActionComment('')
      setEditingTask(false)
      setPage('task')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível abrir o histórico da tarefa.')
    } finally {
      setBusy(false)
    }
  }

  async function runQAEnvironmentAction(action: 'start' | 'stop' | 'restart') {
    if (!selectedTask) return
    setBusy(true); setMessage('')
    try {
      const result = await api<QAEnvironment>(`/api/tasks/${selectedTask.id}/environment/${action}`, { method: 'POST' })
      setQAEnvironment(result)
      if (action === 'stop') setQALogs('')
      else setQALogs((await api<{ logs: string }>(`/api/tasks/${selectedTask.id}/environment/logs`)).logs)
      setMessage(action === 'stop' ? 'Ambiente de QA parado.' : 'Ambiente saudável e disponível para revisão.')
    } catch (error) { setMessage(error instanceof Error ? error.message : 'Não foi possível atualizar o ambiente de QA.') }
    finally { setBusy(false) }
  }

  async function refreshQALogs() {
    if (!selectedTask) return
    try { setQALogs((await api<{ logs: string }>(`/api/tasks/${selectedTask.id}/environment/logs`)).logs) }
    catch (error) { setMessage(error instanceof Error ? error.message : 'Não foi possível carregar os logs.') }
  }

  async function createTaskPR() {
    if (!selectedTask) return
    setBusy(true); setMessage('')
    try {
      const pull = await api<TaskPullRequest>(`/api/tasks/${selectedTask.id}/pull-request`, { method: 'POST' })
      setTaskPullRequest(pull)
      setSelectedTask(await api<Task>(`/api/tasks/${selectedTask.id}`))
      setMessage('Pull request criado e vinculado à tarefa.')
    } catch (error) { setMessage(error instanceof Error ? error.message : 'Não foi possível criar o pull request.') }
    finally { setBusy(false) }
  }

  async function refreshChecks() {
    if (!selectedTask) return
    setBusy(true); setMessage('')
    try {
      const result = await api<{ status: string; checks: NonNullable<TaskPullRequest['checks']> }>(`/api/tasks/${selectedTask.id}/pull-request/checks`, { method: 'POST' })
      setPullRequestChecks(result.status)
      setTaskPullRequest((current) => current ? { ...current, checks: result.checks } : current)
      setSelectedTask(await api<Task>(`/api/tasks/${selectedTask.id}`))
    } catch (error) { setMessage(error instanceof Error ? error.message : 'Não foi possível consultar os checks.') }
    finally { setBusy(false) }
  }

  async function submitAccess(event: FormEvent<HTMLFormElement>, isSetup: boolean) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const payload = isSetup
      ? { username: form.get('username'), displayName: form.get('displayName'), password: form.get('password') }
      : { username: form.get('username'), password: form.get('password') }
    setBusy(true)
    setMessage('')
    try {
      const user = await api<User>(isSetup ? '/api/setup/admin' : '/api/login', {
        method: 'POST', body: JSON.stringify(payload),
      })
      await enterFactory(user)
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível entrar.')
    } finally {
      setBusy(false)
    }
  }

  async function createUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setBusy(true)
    setMessage('')
    try {
      await api<User>('/api/users', {
        method: 'POST',
        body: JSON.stringify({
          username: form.get('username'), displayName: form.get('displayName'),
          password: form.get('password'), role: form.get('role'),
        }),
      })
      setUsers(await api<User[]>('/api/users'))
      setShowCreate(false)
      setMessage('Conta criada. A pessoa já pode entrar com a senha definida.')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível criar a conta.')
    } finally {
      setBusy(false)
    }
  }

  async function updateUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!editing) return
    const form = new FormData(event.currentTarget)
    const password = String(form.get('password') ?? '')
    const update: Record<string, string> = {
      username: String(form.get('username') ?? ''),
      displayName: String(form.get('displayName') ?? ''),
      role: String(form.get('role') ?? ''),
    }
    if (password) update.password = password
    setBusy(true)
    setMessage('')
    try {
      await api<User>(`/api/users/${editing.id}`, { method: 'PATCH', body: JSON.stringify(update) })
      setUsers(await api<User[]>('/api/users'))
      setEditing(null)
      setMessage('Alterações salvas.')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível salvar as alterações.')
    } finally {
      setBusy(false)
    }
  }

  async function openUserManagement() {
    setBusy(true)
    setMessage('')
    try {
      setUsers(await api<User[]>('/api/users'))
      setPage('users')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível carregar as contas.')
    } finally {
      setBusy(false)
    }
  }

  async function createProject(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setBusy(true)
    setMessage('')
    try {
      await api<Project>('/api/projects', { method: 'POST', body: JSON.stringify({
        name: form.get('name'), description: form.get('description'), requirements: form.get('requirements'),
      }) })
      setProjects(await api<Project[]>('/api/projects'))
      setShowProjectForm(false)
      setMessage('Projeto criado e associado ao seu workspace.')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível criar o projeto.')
    } finally {
      setBusy(false)
    }
  }

  async function createTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedProject) return
    const form = new FormData(event.currentTarget)
    setBusy(true)
    setMessage('')
    try {
      await api<Task>(`/api/projects/${selectedProject.id}/tasks`, { method: 'POST', body: JSON.stringify({
        title: form.get('title'), description: form.get('description'),
      }) })
      setTasks(await api<Task[]>(`/api/projects/${selectedProject.id}/tasks`))
      setShowTaskForm(false)
      setMessage('Tarefa criada no estado Aberta.')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível criar a tarefa.')
    } finally {
      setBusy(false)
    }
  }

  async function saveTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedTask || !selectedProject) return
    const form = new FormData(event.currentTarget)
    setBusy(true)
    setMessage('')
    try {
      const updated = await api<Task>(`/api/tasks/${selectedTask.id}`, { method: 'PATCH', body: JSON.stringify({
        title: form.get('title'), description: form.get('description'),
      }) })
      setSelectedTask(updated)
      setTasks(await api<Task[]>(`/api/projects/${selectedProject.id}/tasks`))
      setEditingTask(false)
      setMessage('Tarefa atualizada.')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível atualizar a tarefa.')
    } finally {
      setBusy(false)
    }
  }

  async function performTaskAction(action: string, needsComment = false, needsEvidence = false) {
    if (!selectedTask || !selectedProject) return
    if (needsComment && !actionComment.trim()) {
      setMessage('Adicione um comentário para registrar essa decisão.')
      return
    }
    if (needsEvidence && !qaEvidence.trim()) { setMessage('Registre a evidência da revisão de QA.'); return }
    setBusy(true)
    setMessage('')
    try {
      const updated = await api<Task>(`/api/tasks/${selectedTask.id}/actions`, {
        method: 'POST', body: JSON.stringify({ action, comment: actionComment, evidence: qaEvidence }),
      })
      setSelectedTask(updated)
      setTasks(await api<Task[]>(`/api/projects/${selectedProject.id}/tasks`))
      setEvents(await api<TaskEvent[]>(`/api/tasks/${updated.id}/events`))
      setSpecifications(await api<TaskSpecification[]>(`/api/tasks/${updated.id}/specifications`))
      setImplementations(await api<TaskImplementation[]>(`/api/tasks/${updated.id}/implementations`))
      setActionComment('')
      setQAEvidence('')
      setMessage('Tarefa atualizada e decisão registrada no histórico.')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'A ação não foi concluída.')
    } finally {
      setBusy(false)
    }
  }

  async function deactivateUser(user: User) {
    if (confirmingId !== user.id) {
      setConfirmingId(user.id)
      return
    }
    setBusy(true)
    setMessage('')
    try {
      await api<void>(`/api/users/${user.id}/deactivate`, { method: 'POST' })
      setUsers(await api<User[]>('/api/users'))
      setConfirmingId(null)
      if (editing?.id === user.id) setEditing(null)
      setMessage(`A conta de ${user.displayName} foi desativada.`)
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível desativar a conta.')
    } finally {
      setBusy(false)
    }
  }

  async function signOut() {
    setBusy(true)
    try {
      await api<void>('/api/logout', { method: 'POST' })
      setCurrentUser(null)
      setUsers([])
      setProjects([])
      setSelectedProject(null)
      setTasks([])
      setSelectedTask(null)
      setEvents([])
      setSpecifications([])
      setSpecificationArtifact(null)
      setImplementations([])
      setEditing(null)
      setMessage('Sessão encerrada.')
      setPage('login')
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível encerrar a sessão.')
    } finally {
      setBusy(false)
    }
  }

  const actionButtons: { action: string; label: string; comment?: boolean; evidence?: boolean }[] = []
  if (selectedTask && canOrganize(currentUser?.role) && selectedTask.state === 'Aberta') {
    actionButtons.push({ action: 'mark_ready', label: 'Marcar como pronta' })
  }
  if (selectedTask && canOrganize(currentUser?.role) && selectedTask.state === 'Ajustes nas especificações') {
    actionButtons.push({ action: 'start_specification', label: 'Iniciar nova rodada de especificação' })
  }
  if (selectedTask && canOrganize(currentUser?.role) && selectedTask.state === 'Pronta' && selectedTask.specificationStatus === 'failed') {
    actionButtons.push({ action: 'start_specification', label: 'Tentar gerar especificação novamente' })
  }
  if (selectedTask?.state === 'Aguardando aprovação das especificações' && (currentUser?.role === 'PO' || currentUser?.role === 'Admin')) {
    actionButtons.push({ action: 'approve_specification', label: 'Aprovar especificação' })
    actionButtons.push({ action: 'return_specification', label: 'Devolver para ajustes', comment: true })
  }
  if (selectedTask?.state === 'Aguardando QA' && (currentUser?.role === 'QA' || currentUser?.role === 'Admin')) {
    actionButtons.push({ action: 'qa_started', label: 'Iniciar revisão de QA' })
  }
  if (selectedTask?.state === 'Em QA' && (currentUser?.role === 'QA' || currentUser?.role === 'Admin')) {
    actionButtons.push({ action: 'approve_qa', label: 'Aprovar QA', comment: true, evidence: true })
    actionButtons.push({ action: 'reject_qa', label: 'Reprovar e pedir correção', comment: true, evidence: true })
  }
  if (selectedTask?.state === 'Correção necessária' && (currentUser?.role === 'Desenvolvedor' || currentUser?.role === 'Admin')) {
    actionButtons.push({ action: 'start_correction', label: 'Iniciar correção' })
  }
  if (selectedTask?.state === 'Em desenvolvimento' && selectedTask.implementationStatus === 'failed' && (currentUser?.role === 'Desenvolvedor' || currentUser?.role === 'Admin')) {
    actionButtons.push({ action: 'implementation_started', label: 'Tentar implementação novamente' })
  }
  if (selectedTask?.state === 'Pronta para merge' && canMerge(currentUser?.role)) {
    actionButtons.push({ action: 'merge', label: 'Registrar merge' })
  }
  if (selectedTask?.state === 'Bloqueada' && currentUser?.role === 'Admin') {
    actionButtons.push({ action: 'unblock', label: 'Retomar tarefa' })
  }
  if (selectedTask && selectedTask.state !== 'Bloqueada' && selectedTask.state !== 'Concluída' && currentUser?.role === 'Admin') {
    actionButtons.push({ action: 'block', label: 'Bloquear tarefa', comment: true })
  }

  if (page === 'loading') {
    return <main className="access-shell"><p role="status">Conferindo o acesso à fábrica…</p></main>
  }

  if (page === 'setup' || page === 'login') {
    const isSetup = page === 'setup'
    return (
      <main className="access-shell">
        <a className="brand" href="/" aria-label="Fábrica, início">
          <span className="brand-mark" aria-hidden="true">F</span><span>fábrica</span>
        </a>
        <section className="access-panel" aria-labelledby="access-title">
          <p className="eyebrow">{isSetup ? 'PRIMEIRO ACESSO' : 'AMBIENTE LOCAL'}</p>
          <h1 id="access-title">{isSetup ? 'Configure a conta Admin.' : 'Entre na fábrica.'}</h1>
          <p className="panel-intro">
            {isSetup
              ? 'Crie a conta que administrará usuários e o fluxo de trabalho.'
              : 'Use sua conta individual para acompanhar os projetos da equipe.'}
          </p>
          <form className="form-stack" onSubmit={(event) => void submitAccess(event, isSetup)}>
            <label>Usuário
              <input name="username" autoComplete="username" minLength={3} maxLength={64} required />
            </label>
            {isSetup && <label>Nome de exibição
              <input name="displayName" autoComplete="name" maxLength={120} required />
            </label>}
            <label>Senha
              <input name="password" type="password" autoComplete={isSetup ? 'new-password' : 'current-password'} minLength={12} maxLength={72} required />
              {isSetup && <span className="field-hint">Use de 12 a 72 caracteres.</span>}
            </label>
            {message && <p className="notice notice--error" role="alert">{message}</p>}
            <button className="button button--primary" type="submit" disabled={busy}>
              {busy ? 'Aguarde…' : isSetup ? 'Criar conta Admin' : 'Entrar'}
            </button>
          </form>
        </section>
        <p className="access-note">A fábrica fica disponível somente nesta máquina.</p>
      </main>
    )
  }

  if (page === 'error') {
    return <main className="access-shell"><p className="notice notice--error" role="alert">{message}</p></main>
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <a className="brand" href="/" aria-label="Fábrica, início">
          <span className="brand-mark" aria-hidden="true">F</span><span>fábrica</span>
        </a>
        <div className="account-bar">
          <div className="account-identity"><span className="account-name">{currentUser?.displayName}</span><span className="account-role">{currentUser?.role}</span></div>
          <button className="button button--quiet" type="button" onClick={() => void signOut()} disabled={busy}>Sair</button>
        </div>
      </header>
      <nav className="app-nav" aria-label="Navegação principal">
        <button className={page !== 'users' ? 'nav-link nav-link--active' : 'nav-link'} type="button" onClick={() => { setPage('projects'); setMessage('') }}>Projetos</button>
        {currentUser?.role === 'Admin' && <button className={page === 'users' ? 'nav-link nav-link--active' : 'nav-link'} type="button" onClick={() => void openUserManagement()}>Usuários</button>}
      </nav>

      {page === 'users' ? (
        <section className="users-page" aria-labelledby="users-title">
          <div className="page-heading">
            <div><p className="eyebrow">ADMINISTRAÇÃO</p><h1 id="users-title">Contas da fábrica</h1>
              <p className="panel-intro">Gerencie acessos e defina o papel de cada pessoa no fluxo.</p></div>
            <button className="button button--primary" type="button" onClick={() => { setShowCreate(!showCreate); setEditing(null); setMessage('') }}>
              {showCreate ? 'Fechar formulário' : 'Adicionar usuário'}
            </button>
          </div>

          {message && <p className={message.includes('não') || message.includes('could not') ? 'notice notice--error' : 'notice'} role="status">{message}</p>}

          {showCreate && <section className="editor-panel" aria-labelledby="new-user-title">
            <div className="editor-heading"><h2 id="new-user-title">Nova conta</h2><span>Campos obrigatórios</span></div>
            <form className="user-form" onSubmit={(event) => void createUser(event)}>
              <label>Usuário<input name="username" autoComplete="off" minLength={3} maxLength={64} required /></label>
              <label>Nome de exibição<input name="displayName" maxLength={120} required /></label>
              <label>Perfil<select name="role" defaultValue="Desenvolvedor">{roles.map((role) => <option key={role}>{role}</option>)}</select></label>
              <label>Senha inicial<input name="password" type="password" autoComplete="new-password" minLength={12} maxLength={72} required /></label>
              <div className="form-actions"><button className="button button--primary" type="submit" disabled={busy}>{busy ? 'Salvando…' : 'Criar conta'}</button></div>
            </form>
          </section>}

          <div className="table-wrap">
            <table>
              <thead><tr><th scope="col">Pessoa</th><th scope="col">Perfil</th><th scope="col">Estado</th><th scope="col"><span className="visually-hidden">Ações</span></th></tr></thead>
              <tbody>{users.map((user) => <tr key={user.id} className={!user.active ? 'user-row user-row--inactive' : 'user-row'}>
                <td><span className="user-name">{user.displayName}</span><span className="user-handle">@{user.username}</span></td>
                <td><span className="role-tag">{user.role}</span></td>
                <td><span className={user.active ? 'state-tag state-tag--active' : 'state-tag'}>{user.active ? 'Ativa' : 'Desativada'}</span></td>
                <td className="row-actions">
                  <button className="button button--quiet" type="button" onClick={() => { setEditing(user); setShowCreate(false); setConfirmingId(null); setMessage('') }}>Editar</button>
                  {user.active && user.id !== currentUser?.id && (confirmingId === user.id
                    ? <><button className="button button--danger" type="button" onClick={() => void deactivateUser(user)} disabled={busy}>Confirmar</button><button className="text-button" type="button" onClick={() => setConfirmingId(null)}>Cancelar</button></>
                    : <button className="text-button" type="button" onClick={() => void deactivateUser(user)}>Desativar</button>)}
                </td>
              </tr>)}</tbody>
            </table>
            {users.length === 0 && <p className="empty-state">Nenhuma conta cadastrada.</p>}
          </div>

          {editing && <section className="editor-panel" aria-labelledby="edit-user-title">
            <div className="editor-heading"><h2 id="edit-user-title">Editar {editing.displayName}</h2><button className="text-button" type="button" onClick={() => setEditing(null)}>Cancelar</button></div>
            <form className="user-form" onSubmit={(event) => void updateUser(event)}>
              <label>Usuário<input name="username" defaultValue={editing.username} minLength={3} maxLength={64} required /></label>
              <label>Nome de exibição<input name="displayName" defaultValue={editing.displayName} maxLength={120} required /></label>
              <label>Perfil<select name="role" defaultValue={editing.role}>{roles.map((role) => <option key={role}>{role}</option>)}</select></label>
              <label>Nova senha <span className="optional-label">(opcional)</span><input name="password" type="password" autoComplete="new-password" minLength={12} maxLength={72} /></label>
              <div className="form-actions"><button className="button button--primary" type="submit" disabled={busy}>{busy ? 'Salvando…' : 'Salvar alterações'}</button></div>
            </form>
          </section>}
          <p className="retention-note">Contas desativadas permanecem no histórico e não podem iniciar sessão.</p>
        </section>
      ) : page === 'projects' ? (
        <section className="projects-page" aria-labelledby="projects-title">
          <div className="page-heading">
            <div><h1 id="projects-title">Projetos da fábrica</h1><p className="panel-intro">Cada projeto tem seu workspace e um fluxo de tarefas próprio.</p></div>
            {canOrganize(currentUser?.role) && <button className="button button--primary" type="button" onClick={() => { setShowProjectForm(!showProjectForm); setMessage('') }}>
              {showProjectForm ? 'Fechar formulário' : 'Criar projeto'}
            </button>}
          </div>
          {message && <p className="notice" role="status">{message}</p>}
          {showProjectForm && <section className="editor-panel" aria-labelledby="new-project-title">
            <div className="editor-heading"><h2 id="new-project-title">Novo projeto</h2><span>O nome também define a pasta do workspace</span></div>
            <form className="project-form" onSubmit={(event) => void createProject(event)}>
              <label>Nome do projeto<input name="name" autoComplete="off" maxLength={64} placeholder="ex.: portal-cliente" required /></label>
              <label>Descrição<textarea name="description" rows={2} maxLength={10000} /></label>
              <label>Stack e requisitos<textarea name="requirements" rows={4} maxLength={20000} placeholder="Tecnologias, integrações e critérios importantes" /></label>
              <div className="form-actions"><button className="button button--primary" type="submit" disabled={busy}>{busy ? 'Criando…' : 'Criar projeto'}</button></div>
            </form>
          </section>}
          {projects.length ? <ul className="project-list">
            {projects.map((project) => <li key={project.id}>
              <button className="project-row" type="button" onClick={() => void openProject(project)} disabled={busy}>
                <span className="project-row-main"><span className="project-name">{project.name}</span><span className="project-description">{project.description || 'Sem descrição ainda.'}</span></span>
                <span className="project-workspace">{project.workspacePath}</span><span className="project-open">Abrir projeto</span>
              </button>
            </li>)}
          </ul> : <div className="empty-state empty-state--large"><h2>Nenhum projeto ainda</h2><p>{canOrganize(currentUser?.role) ? 'Crie um projeto para definir seu workspace e começar a organizar tarefas.' : 'PO ou Scrum Master pode criar o primeiro projeto.'}</p></div>}
        </section>
      ) : page === 'project' && selectedProject ? (
        <section className="project-page" aria-labelledby="project-title">
          <button className="back-link" type="button" onClick={() => { setPage('projects'); setMessage('') }}>← Todos os projetos</button>
          <div className="page-heading project-heading">
            <div><h1 id="project-title">{selectedProject.name}</h1><p className="panel-intro">{selectedProject.description || 'Este projeto ainda não tem descrição.'}</p></div>
            {canOrganize(currentUser?.role) && <button className="button button--primary" type="button" onClick={() => { setShowTaskForm(!showTaskForm); setMessage('') }}>
              {showTaskForm ? 'Fechar formulário' : 'Criar tarefa'}
            </button>}
          </div>
          <p className="workspace-line"><span>Workspace</span><code>{selectedProject.workspacePath}</code></p>
          {message && <p className="notice" role="status">{message}</p>}
          {showTaskForm && <section className="editor-panel" aria-labelledby="new-task-title">
            <div className="editor-heading"><h2 id="new-task-title">Nova tarefa</h2><span>Uma tarefa nova começa como Aberta</span></div>
            <form className="project-form" onSubmit={(event) => void createTask(event)}>
              <label>Título<input name="title" maxLength={200} required /></label>
              <label>Descrição<textarea name="description" rows={3} maxLength={10000} /></label>
              <div className="form-actions"><button className="button button--primary" type="submit" disabled={busy}>{busy ? 'Criando…' : 'Criar tarefa'}</button></div>
            </form>
          </section>}
          <div className="board-heading"><h2>Quadro de tarefas</h2><span>{tasks.length} {tasks.length === 1 ? 'tarefa' : 'tarefas'}</span></div>
          {tasks.length ? <div className="task-board">
            {taskColumns.map((column) => {
              const columnTasks = tasks.filter((task) => column.states.includes(task.state))
              return <section className="task-column" key={column.title} aria-label={`${column.title}: ${columnTasks.length} tarefas`}>
                <div className="task-column-heading"><h3>{column.title}</h3><span>{columnTasks.length}</span></div>
                <div className="task-column-body">{columnTasks.length ? columnTasks.map((task) => <button className="task-card" type="button" key={task.id} onClick={() => void openTask(task)}>
                  <span className="task-card-title">{task.title}</span><span className="task-state">{task.state}</span>
                </button>) : <p className="column-empty">Sem tarefas</p>}</div>
              </section>
            })}
          </div> : <div className="empty-state empty-state--large"><h2>O quadro está vazio</h2><p>{canOrganize(currentUser?.role) ? 'Crie a primeira tarefa para iniciar o fluxo do projeto.' : 'Uma tarefa aparecerá aqui quando for criada.'}</p></div>}
        </section>
      ) : page === 'task' && selectedTask && selectedProject ? (
        <section className="task-detail-page" aria-labelledby="task-title">
          <button className="back-link" type="button" onClick={() => { setPage('project'); setMessage('') }}>← {selectedProject.name}</button>
          <div className="task-detail-heading">
            <div><p className="task-project-name">{selectedProject.name}</p><h1 id="task-title">{selectedTask.title}</h1></div>
            <span className="task-state task-state--large">{selectedTask.state}</span>
          </div>
          <div className="task-detail-grid">
            <div className="task-main-column">
              <section className="next-action" aria-labelledby="next-action-title">
                <h2 id="next-action-title">Próxima ação</h2>
                <p>{nextTaskAction(selectedTask, currentUser?.role)}</p>
              </section>
              {specifications.length > 0 && <section className="specification-panel" aria-labelledby="specification-title">
                <div className="section-heading"><h2 id="specification-title">Especificação da tarefa</h2><span>Rodada {specifications.at(-1)?.round} · {phaseStatus(specifications.at(-1)?.status ?? '')}</span></div>
                <label className="artifact-picker">Artefato
                  <select value={specificationArtifact?.path ?? ''} onChange={(event) => {
                    const latest = specifications.at(-1)
                    const path = event.target.value
                    if (!selectedTask || !latest || !path) return
                    void api<SpecificationArtifact>(`/api/tasks/${selectedTask.id}/specifications/${latest.round}/artifact?path=${encodeURIComponent(path)}`).then(setSpecificationArtifact).catch((error) => setMessage(error instanceof Error ? error.message : 'Não foi possível abrir o artefato.'))
                  }}>
                    {specifications.at(-1)?.artifactPaths.map((path) => <option key={path} value={path}>{path}</option>)}
                  </select>
                </label>
                {specificationArtifact && <><p className="artifact-path">{specificationArtifact.path}</p><pre className="artifact-preview">{specificationArtifact.content}</pre></>}
              </section>}
              {implementations.length > 0 && <section className="implementation-panel" aria-labelledby="implementation-title">
                <div className="section-heading"><h2 id="implementation-title">Execuções de implementação</h2><span>{implementations.length} tentativas</span></div>
                <ol className="implementation-list">{implementations.map((run) => <li key={run.id}>
                  <div><strong>Tentativa {run.attempt}</strong><span className={`run-status run-status--${run.status}`}>{phaseStatus(run.status)}</span></div>
                  <p>{run.summary}</p>
                  {run.changedFiles.length > 0 && <ul>{run.changedFiles.map((path) => <li key={path}>{path}</li>)}</ul>}
                  <time dateTime={run.createdAt}>{new Date(run.createdAt).toLocaleString('pt-BR')}</time>
                </li>)}</ol>
              </section>}
              <section className="qa-environment-panel" aria-labelledby="qa-environment-title">
                <div className="section-heading"><h2 id="qa-environment-title">Ambiente de QA</h2><span>{qaEnvironment?.status === 'healthy' ? 'Disponível' : qaEnvironment?.status === 'unhealthy' ? 'Health check falhou' : qaEnvironment?.status === 'starting' ? 'Iniciando' : 'Parado'}</span></div>
                {qaEnvironment?.url && <p className="qa-environment-url"><a href={qaEnvironment.url} target="_blank" rel="noreferrer">Abrir revisão local ↗</a><span>127.0.0.1 · porta {qaEnvironment.port}</span></p>}
                {selectedTask.state === 'Aguardando QA' || selectedTask.state === 'Em QA' ? <div className="decision-buttons">
                  {qaEnvironment?.status === 'starting' ? <p role="status">A aplicação está iniciando. O link aparecerá após o health check.</p> : qaEnvironment?.status === 'healthy' ? <><button className="button button--primary" type="button" disabled={busy} onClick={() => void runQAEnvironmentAction('restart')}>Reiniciar</button><button className="button button--quiet" type="button" disabled={busy} onClick={() => void runQAEnvironmentAction('stop')}>Parar ambiente</button></> : <button className="button button--primary" type="button" disabled={busy} onClick={() => void runQAEnvironmentAction('start')}>{busy ? 'Iniciando e verificando…' : 'Iniciar ambiente de revisão'}</button>}
                  {qaEnvironment?.status === 'healthy' && <button className="text-button text-button--neutral" type="button" onClick={() => void refreshQALogs()}>Atualizar logs</button>}
                </div> : <p className="empty-state">O ambiente aparece quando a implementação estiver pronta para QA.</p>}
                {qaLogs && <pre className="qa-environment-logs" aria-label="Logs do serviço de QA">{qaLogs}</pre>}
              </section>
              {(selectedTask.state === 'Aguardando PR' || selectedTask.state === 'PR aberto' || selectedTask.state === 'Pronta para merge' || taskPullRequest) && <section className="pull-request-panel" aria-labelledby="pull-request-title">
                <div className="section-heading"><h2 id="pull-request-title">Pull request</h2><span>{pullRequestChecks ? `Checks: ${phaseStatus(pullRequestChecks)}` : taskPullRequest ? 'Vinculado' : 'Aguardando criação'}</span></div>
                {taskPullRequest ? <><p><a href={taskPullRequest.url} target="_blank" rel="noreferrer">PR #{taskPullRequest.number} · abrir no GitHub ↗</a></p><p className="pull-request-branch">Branch {taskPullRequest.branch}</p><button className="button button--quiet" type="button" disabled={busy} onClick={() => void refreshChecks()}>Consultar checks obrigatórios</button>{taskPullRequest.checks?.length ? <ul className="pull-request-checks">{taskPullRequest.checks.map((check) => <li key={check.name}><span>{check.name}</span><strong>{phaseStatus(check.state.toLowerCase())}</strong></li>)}</ul> : null}</> : selectedTask.state === 'Aguardando PR' && canMerge(currentUser?.role) ? <button className="button button--primary" type="button" disabled={busy} onClick={() => void createTaskPR()}>{busy ? 'Criando PR…' : 'Criar pull request'}</button> : <p className="empty-state">O PR só pode ser criado depois da aprovação do QA.</p>}
              </section>}
              {message && <p className="notice" role="status">{message}</p>}
              {editingTask ? <section className="editor-panel" aria-labelledby="edit-task-title">
                <div className="editor-heading"><h2 id="edit-task-title">Editar tarefa</h2><button className="text-button" type="button" onClick={() => setEditingTask(false)}>Cancelar</button></div>
                <form className="project-form" onSubmit={(event) => void saveTask(event)}>
                  <label>Título<input name="title" defaultValue={selectedTask.title} maxLength={200} required /></label>
                  <label>Descrição<textarea name="description" defaultValue={selectedTask.description} rows={4} maxLength={10000} /></label>
                  <div className="form-actions"><button className="button button--primary" type="submit" disabled={busy}>{busy ? 'Salvando…' : 'Salvar tarefa'}</button></div>
                </form>
              </section> : <section className="task-description" aria-labelledby="description-title">
                <div className="section-heading"><h2 id="description-title">Descrição</h2>{canOrganize(currentUser?.role) && <button className="text-button text-button--neutral" type="button" onClick={() => setEditingTask(true)}>Editar</button>}</div>
                <p>{selectedTask.description || 'Esta tarefa ainda não tem descrição.'}</p>
              </section>}
              <section className="history-section" aria-labelledby="history-title">
                <div className="section-heading"><h2 id="history-title">Histórico da tarefa</h2><span>{events.length} registros</span></div>
                {events.length ? <ol className="event-list">{events.map((event) => <li className={`event-row event-row--${event.type}`} key={event.id}>
                  <div className="event-marker" aria-hidden="true" />
                  <div className="event-content"><div className="event-heading"><strong>{event.actorName}</strong><span>{event.actorRole}</span><time dateTime={event.occurredAt}>{new Date(event.occurredAt).toLocaleString('pt-BR')}</time></div>
                    <p className="event-action">{eventActionLabel(event.action)} · {eventTypeLabel(event.type)}</p>
                    {(event.fromState !== event.toState) && <p className="event-transition">{event.fromState} <span aria-hidden="true">→</span> {event.toState}</p>}
                    {event.comment && <p className="event-comment">{event.comment}</p>}
                    {event.reason && <p className="event-reason">{event.reason}</p>}
                  </div>
                </li>)}</ol> : <p className="empty-state">As decisões e mudanças desta tarefa aparecerão aqui.</p>}
              </section>
            </div>
            <aside className="task-side-column" aria-label="Estado e decisões">
              <section className="signal-panel"><h2>Fases</h2><dl className="signal-list">
                <div><dt>Especificação</dt><dd>{phaseStatus(selectedTask.specificationStatus)}</dd></div>
                <div><dt>Implementação</dt><dd>{phaseStatus(selectedTask.implementationStatus)}</dd></div>
                <div><dt>QA</dt><dd>{phaseStatus(selectedTask.qaStatus)}</dd></div>
                <div><dt>Pull request</dt><dd>{phaseStatus(selectedTask.prStatus)}</dd></div>
                <div><dt>Checks</dt><dd>{phaseStatus(selectedTask.checksStatus)}</dd></div>
              </dl></section>
              {actionButtons.length > 0 && <section className="decision-panel" aria-labelledby="decision-title">
                <h2 id="decision-title">Ações disponíveis</h2>
                {actionButtons.some((item) => item.comment) && <label className="decision-comment">Comentário para o histórico<textarea value={actionComment} onChange={(event) => setActionComment(event.target.value)} rows={3} maxLength={4000} /></label>}
                {actionButtons.some((item) => item.evidence) && <label className="decision-comment">Evidência de QA (URL, observação ou referência)<textarea value={qaEvidence} onChange={(event) => setQAEvidence(event.target.value)} rows={3} maxLength={4000} /></label>}
                <div className="decision-buttons">{actionButtons.map((item) => <button className={item.action === 'block' ? 'button button--danger' : 'button button--primary'} type="button" key={item.action} disabled={busy} onClick={() => void performTaskAction(item.action, item.comment, item.evidence)}>{item.label}</button>)}</div>
              </section>}
            </aside>
          </div>
        </section>
      ) : null}
      <footer className="footer"><span>Fábrica de Software Agêntica</span><span>Go · React · PostgreSQL</span></footer>
    </main>
  )
}

export default App
