# Changelog

## [Unreleased]

## [0.23.4] - 2026-10-25

### Fixed
- **恢复后长时间没有下载进度**：完成断点与清单已一致的记录直接跳过，避免每次恢复都从头重读已下载的大文件。清单缺失或记录不一致时，仍核验本地文件后补记。
- **已有文件检查可暂停**：需要重新核验的文件在读取间检查取消信号，避免暂停操作等待整份大文件校验完成。

### Added
- **恢复阶段提示**：任务页显示连接 Telegram、核对已有下载记录等阶段，开始文件下载后切换为文件进度。

## [0.23.3] - 2026-10-25

### Fixed
- **暂停后无法恢复**：Telegram 客户端会把取消返回为无错误；任务收尾现在先核对调用方上下文，避免把暂停/取消误标为完成并隐藏恢复按钮。
- **任务完成状态兜底**：即使底层返回无错误，已记录文件失败、清单写入失败或仍有未处理项目时，也不会再把任务标为完成。磁盘保护主动停止并带提示的行为保留。
- **历史错误状态恢复**：启动时把没有主动停止提示、仍含未处理项目的已完成任务恢复为暂停，失败项仍存在的任务恢复为失败；保留任务范围、已完成文件、跳过项和断点。

### Validation
- Go 构建、全量测试、静态检查、前端类型检查/生产构建、8 项前端回归测试及 Windows amd64 生产构建通过。新增回归复现取消被吞掉后误报成功，并验证恢复原任务范围及已完成进度；没有对用户运行中的客户端执行真实下载或重启操作。

## [0.23.2] - 2026-10-25

### Added
- **下载磁盘空间保护**：新增磁盘使用率上限（1%–99%，默认 90%），按下载目录所在磁盘检查整体使用率及用户可用空间；写入前和下载期间检测，超限时等待并发文件收尾并提示原因。
- **自动完成清单**：未选择清单时，在下载目录按任务和来源对话生成 JSONL 清单；选择清单时继续追加。仅记录完整保存或经来源与摘要核对的完成项，暂停、恢复及重启后沿用同一清单。

### Fixed
- **自动清单会话耗尽**：自动清单不再占用应用全局导入清单的 256 个缓存名额，任务收尾后释放自动清单会话。
- **清单补记与错误状态**：清单写入失败后重试，会核对本地完成文件并补记清单；磁盘停止不会将文件保存失败伪装为成功，也不会覆盖原有执行错误。
- **用户取消与磁盘检测竞争**：已取消的任务不再因同时触发磁盘检测而转为完成状态。
- **内置教程**：同步应用内的磁盘上限、自动清单与重试补记说明。
- **Unix 排他发布**：Linux/macOS 优先使用不覆盖现有目标的原子重命名，通过受限根目录打开的父目录描述符执行；不支持时保留安全的硬链接降级，避免直接依赖硬链接导致部分文件系统发布失败，也不引入“先检查再普通重命名”的覆盖竞态。
- **清单审查缓存**：容量达到 8 条后仅淘汰创建时间最早的一条，保留其余未过期结果；30 分钟有效期和容量上限不变。
- **已确认重置恢复**：保存已验证的保护目录与计划版本，使已确认重置可在配置或任务库损坏后恢复。当前已知的目录重叠仍优先拒绝；没有安全范围、旧记录缺范围或保护路径非法时不盲删，也不执行记录指定的任意目标。
- **Windows 云目录分类**：按具体重解析 tag 放行 Cloud Filter 占位目录，继续拒绝符号链接、联接、卷挂载点、未知 tag 及查询失败。
- **评论根消息**：允许频道评论参数指向讨论串根消息本身，保留对其他讨论串及 peer 来源的校验。
- **缩略图并发缓存**：在取得锁后复查已完成的缓存写入，避免 inflight 条目刚被移除时重复拉取。

### Changed
- **提交排除规则**：补充本地工作目录、缓存、临时文件、Go 测试二进制、调试符号、覆盖率/性能分析文件，以及运行数据库及其 SQLite 边车文件的忽略规则；源码、测试和构建输入继续纳入版本控制。

### Validation
- 0.23.2：Go 构建、全量测试、静态检查、前端类型检查/生产构建、8 项前端回归和 Windows amd64 生产构建通过；Linux/amd64 与 macOS/arm64 引擎测试交叉编译通过。合成数据界面验证覆盖上限保存、新任务提示及磁盘停止通知；真实 Telegram 下载和真实磁盘填满未实测。
- Go 构建、全量测试、静态检查和 Windows amd64 生产构建通过；既有缩略图 singleflight 回归连续执行 30 次通过。
- Linux/amd64、macOS/arm64 引擎与 Unix 专用测试交叉编译通过，尚未在真实 Unix 卷或 OneDrive/iCloud 环境实测。详细结论见 [二次核实与最小修订](doc/audit/2026-10-25-minimal-review-fixes.md)。

## [0.23.1] - 2026-10-25

### Added
- **便携数据与退出策略**：新安装默认使用程序旁的数据目录，兼容旧数据和绝对路径环境变量；WebView、会话、配置与默认缓存有明确位置。增加数据目录实例锁、退出询问、默认缓存清理，以及逐次确认的应用数据重置。
- **统一下载前比对**：单项、选中和整个对话共用确认窗口；区分清单跳过、本地确认存在和预计下载，避免重复计数。新增完整筛选结果扫描、整个对话下载、分页去重、游标检查、取消与数量上限处理。
- **本地已下载清单**：扫描目录并展示原名、大小、消息完整时间与本地候选；重名逐项确认，导出前再次验证，零匹配不生成文件。新清单采用带账号/对话来源的 v3 JSONL，支持完成后同步追加、跨实例写入锁和中断尾记录恢复，并兼容旧 JSON 中有效的确认项。
- **任务与文件操作**：展示成功、失败和跳过计数、原因及筛选；增加选中文件批量重下。重下创建仅含选中消息的新任务，关闭相册展开，保留原任务与断点。
- **可复现构建与回归检查**：增加 Windows 构建脚本、隔离的构建工具依赖、无副作用的绑定生成入口，以及后端和前端回归测试、本地合成数据界面夹具和审查记录。

### Fixed
- **文件覆盖与输出越界**：普通下载保留已有文件，冲突时编号；独立临时文件在长度、刷新和后处理完成后发布。防止并发撞名、路径穿越、系统保留文件名及链接越界；只有重下已完成项允许覆盖登记的最终目标，跳过或失败项继续保留已有同名文件。
- **账号与来源混用**：按账号、peer 命名空间、对话、消息和媒体身份隔离缓存、标记及清单；登录切换与下载/媒体查询互斥，并核对实际登录账号。支持收藏夹、超级群组及消息链接中的评论/话题/单项语义。
- **本地误判与记录丢失**：新增完成文件 SHA-256，自动跳过前检查来源与摘要；删除记录失败时保留可见状态，避免误报成功或重启后记录复活；任务收尾期间阻止不安全的恢复和删除。
- **清理误删与缓存接管**：清理前检查当前及历史下载目录、链接和自定义路径；退出仅清理默认媒体缓存。重置保留下载文件、外部自定义目录和数据根目录未知文件；旧的内部自定义目录未迁移时拒绝重置。自定义非空缓存子目录没有应用归属标记时拒绝接管。
- **日志和后台缓存行为**：修复固定可写探针覆盖文件、预加载空结果误报命中、缓存统计竞争及后台清理生命周期；部分日志 YAML 没有目录时沿用已选路径，不静默改写存储边界。

### Security
- **脚本执行边界**：限制导入和源文件访问，禁止后台 goroutine、文件枚举与不可取消等待；初始化和契约调用均设超时，超时后取消并禁用该脚本的全部契约函数。
- **界面与配置保护**：生产界面增加本地内容策略，性能统计只保存在本地；设置快照不能覆盖账号归属等后端元数据。配置原子保存，新建 Windows 数据目录设置继承访问权限，日志中的代理凭据脱敏。
- **依赖更新**：更新前端锁文件及 Go 的网络、加密、文本、压缩等安全依赖；最低 Go 版本升为 1.26，并锁定兼容当前工具链的 Wails 构建工具依赖。

### Changed
- **版本与说明**：统一版本为 0.23.1，更新前后端契约、设置提示、使用教程、开发环境和审查文档。恢复按完整文件保留进度，未完成文件重下，不宣称支持字节级分片续传。
- **版本控制范围**：仅跟踪源码、测试、构建输入、依赖锁文件和文档。排除成品、构建目录、生成绑定、依赖目录、应用数据和本地构建工具；停止跟踪 Wails 自动生成的 `package.json.md5` 缓存标记。

### Validation
- Go 构建、现有与新增测试、静态检查、前端类型检查/生产构建、8 个前端回归测试，以及 Windows amd64 生产构建通过。
- 模块完整性检查和前端依赖审计通过；生产构建配置下未发现导入包或可达符号受已知 Go 漏洞影响。保留一条涉及未导入 OpenPGP 包的模块级提示，不将其宣称为已修复。
- 原生隔离目录启动、清理助手和合成数据界面流程已检查。真实 Telegram 全流程、macOS/Linux 图形运行及真实大规模压力测试未覆盖；原生画面抓取失败，竞态检测因缺少 gcc 未完成。详细范围见 [审查与验证记录](doc/audit/2026-10-25-patch-hardening.md)。

## [0.23.0] - 2026-09-21 23:56:39

### Added
- **下载速度实时显示**：任务页头部新增全局总速度指示（`pi-bolt` + 速度值，仅总速度 > 0 时显示），任务卡片显示任务级聚合速度（仅运行中），文件行显示单文件实时速度（仅下载中）。
- **后端速度采样与聚合**：`progress` 按前后两次进度事件的字节差/时间差计算单文件速度（字节/秒），`FileEvent` 新增 `speed` 字段（仅 downloading 时携带），done/failed 时清理采样状态；`Task` 维护 `fileSpeeds` 速度聚合缓存，`TaskView` 新增 `speed` 字段。
- **`GetTotalSpeed` 绑定接口**：`Manager.TotalSpeed()` 聚合全部运行中任务的速度，经 `DownloadService.GetTotalSpeed()` 暴露给前端。
- **`fmtSpeed` 格式化工具**：前端 `format.ts` 新增字节/秒 → 人类可读速度（如 `1.5 MB/s`）的格式化函数。

### Changed
- **总速度轮询刷新**：前端任务 store 新增 `totalSpeed` 状态与 `refreshTotalSpeed()`，任务页挂载期间以 1 秒间隔轮询 `GetTotalSpeed`，卸载时清理定时器（避免高频事件推送）。

## [0.22.0] - 2026-09-19 13:33:51

### Added
- **任务页搜索过滤工具栏**：任务列表顶部新增搜索框，按标题/链接/目录/脚本名实时过滤，支持正则（`.*`）与区分大小写（`Aa`）切换按钮，右侧显示「N / M 个任务」匹配计数，无匹配时展示空状态提示。
- **任务文件列表搜索过滤**：展开的任务文件列表（文件数超过 3 个时）支持按文件名/路径搜索，按任务独立维护正则与大小写开关，无匹配时提示「没有匹配的文件」。

### Fixed
- **任务链接为空时渲染崩溃**：`TaskView.urls` 为可空字段，任务标题与 tooltip 渲染改用 `t.urls ?? []` 兜底，避免无链接任务导致页面渲染抛错。
- **非法正则降级处理**：搜索正则非法时不抛错、退化为不过滤，输入框标红并提示「正则表达式无效（已退化为不过滤）」。

### Changed
- **任务页标题辅助函数前置**：`taskTitle` / `taskPercent` 迁移至 `filteredTasks` 计算属性之前定义，避免 setup 编译后引用未初始化绑定（TDZ）导致渲染异常。
- **搜索状态生命周期管理**：任务离开终态或重新运行时，同步清理文件级搜索查询/正则/大小写状态，避免状态缓慢累积。

## [0.21.0] - 2026-09-19 12:43:28

### Added
- **代理模式功能**：设置页新增「使用代理」总开关与代理模式二选一（使用系统代理 / 自定义代理）。系统代理模式读取操作系统环境变量（`HTTPS_PROXY` / `HTTP_PROXY` / `ALL_PROXY`，按序探测首个非空值），自定义模式沿用原有手填地址；关闭后所有网络请求直连。
- **`EffectiveProxy` 统一解析**：后端 `config.EffectiveProxy(settings)` 按模式返回实际生效的代理地址（off→直连；custom→用户地址；system→环境变量），下载引擎、登录流程（会话验证/验证码登录/扫码登录）、常驻聊天连接全部统一改用该函数解析代理。
- **旧配置自动迁移**：历史配置无 `proxyMode` 字段但已填写代理地址的，一次性视为 custom 模式并在保存后固化；无地址的默认 system 模式。
- **对话页 Shift 区间选择**：媒体卡片支持 Shift+点击以最近一次点击为锚点连续区间选择（仅追加不覆盖），复选框与卡片空白区行为一致。
- **任务页失败/取消重试按钮**：失败或已取消任务新增「重试（断点续传）」按钮，复用恢复逻辑继续下载未完成文件。

### Changed
- **设置校验增强**：`ProxyMode` 仅接受 system/custom/off，空值回退 system，非法值保存时即报错；custom 模式下代理地址不能为空。
- **代理设置文档与前端内置教程同步**：`doc/tutorials/intermediate/02-settings.md` 与 `src/frontend/src/content/tutorials/intermediate-settings.md` 重写「网络」章节（使用代理 / 代理模式）。

## [0.20.0] - 2026-08-09 01:52:35

### Added
- **下载前手动重命名文件**：新建任务对话框新增"文件重命名"折叠面板（选集模式且存在媒体项时显示），可为每个选中媒体项指定自定义文件名（不含扩展名），命名优先级为手动重命名 > 脚本 Rename > 默认模板（FE-25）。
- **重命名映射持久化**：`tasks` 表新增 `renames` 列（JSON 序列化的 messageId → 自定义文件名映射，schema 迁移至 v6），任务重启/恢复后重命名规则不丢失。

## [0.19.1] - 2026-08-09 00:39:35

### Security
- **日志页 HTML 白名单净化**：日志行渲染结果经 `sanitizeLogHtml` 白名单校验，仅允许 span/mark 标签及预定义类名（lg-time/lg-level/lv-* 等），`escapeHtml` 补充单引号转义，防止日志内容注入恶意标签与属性。
- **Markdown 渲染 HTML 白名单净化**：`renderMarkdown` 输出经 `sanitizeMarkdownHtml` 标签/属性/类名三重白名单过滤，拦截 `javascript:` 伪协议链接并为 `<a>` 强制追加 `rel="noopener noreferrer"`，防止供应链投毒类 XSS 注入。
- **外部链接协议校验**：`openExternal` 仅放行 http/https 协议，`javascript:` 等伪协议直接拦截；浏览器回退路径补加 `noreferrer`，杜绝新页面通过 `window.opener` 反向访问源页面。
- **性能上报端点限制**：`PerformanceMonitor.enableReporting` 仅接受同源或本地回环地址（localhost/127.0.0.1/::1），其他端点一律拒绝并告警，防止性能指标数据外泄到第三方。

## [0.19.0] - 2026-08-05 22:17:51

### Added
- **媒体卡片消息文本悬停预览**：聊天页媒体卡片悬停 0.5 秒后跟随鼠标位置显示完整消息文本（caption），移出卡片即消失，仿电视平台预览效果。
- **消息内容详情弹窗**：聊天页卡片详情按钮与预览弹窗工具栏"详情"按钮均可打开"消息内容"弹窗，查看完整消息文本并一键复制到剪贴板（剪贴板 API 不可用时回退 textarea 方案）。

### Changed
- **媒体卡片信息区布局调整**：卡片底部信息区改为 flex 布局并限制最小宽度，为消息文本详情按钮腾出空间。
- **表单控件语义增强**：媒体过滤工具栏搜索框/扩展名/大小输入框补充 `name` 属性（经 SearchBox 透传），布局切换下拉补充 `option-label`，提升表单语义与可访问性。

## [0.18.1] - 2026-08-01 13:10:23

### Changed
- **AI 编程代理指南重命名**：`agent.md` 重命名为 `AGENTS.md`（符合业界惯例），内容同步更新（目录结构、后端模块、前端约定、验证要求），README 中的文档索引同步修正。
- **README / README_EN 功能特性同步**：对话浏览补充虚拟滚动与智能预加载、任务管理补充单文件操作、新增媒体预览（Lightbox + 自定义视频播放器）与日志系统（实时浏览/历史查看/搜索过滤/导出）特性描述，快速上手新增日志页与教程页指引。
- **doc/dev-ai 系列文档同步**：README、基础约束、绑定与事件、跨层特性等文档按当前代码实现更新。
- **doc/dev-human 系列文档同步**：`intermediate/02-download-engine.md` 新增 SQLite 持久化层（store/ 四表结构、TaskRepo 接口方法表、WAL 配置、单连接池约束）与单文件操作接口（RedownloadFile / DeleteFileRecord / RevealFileInDir / ResumeTaskWithPending）章节；环境搭建、结构构建、Wails IPC 文档同步更新。
- **doc/tutorials 系列文档同步**：`intermediate/02-settings.md` 新增日志显示（字体/滚动条/最大行数）、存储（缓存目录/视频临时目录/清空缓存）、日志（输出目标/级别/目录/格式模板/滚动策略）章节；安装、下载、过滤重命名、钩子教程同步更新。
- **前端内置教程内容同步**：`src/frontend/src/content/tutorials/` 下 basic-install、intermediate-download、intermediate-settings 与 doc/tutorials 保持同步。

## [0.18.0] - 2026-08-01 11:53:07

### Added
- **任务页单文件操作**：每个文件项新增四个操作按钮——重新下载（从断点移除并重置状态）、打开所在目录（系统文件管理器定位）、删除记录（仅移除数据库记录保留磁盘文件）、删除文件（同时删除磁盘文件）。支持对已完成/失败/已取消任务执行继续下载未完成文件。
- **`RedownloadFile` 接口**：后端 engine/manager 层支持按 `taskID + filePath` 从断点中移除记录并重置文件状态为 downloading，自动重新启动终态任务。
- **`DeleteFileRecord` 接口**：仅删除文件在数据库中的记录，不触碰磁盘文件。
- **`DeleteFile` 接口**：删除单个磁盘文件并从任务文件列表中移除。
- **`RevealFileInDir` 接口**：跨平台在系统文件管理器中打开目录并选中指定文件（Windows explorer /select, macOS open -R, Linux xdg-open 目录）。
- **`ResumeTaskWithPending` 接口**：继续下载任务中所有未完成文件（断点续传），适用于部分文件下载失败后单独恢复剩余项的场景。
- **`DeleteResumeKey` 存储方法**：`TaskRepo` 接口与 SQLite 实现新增按 key 删除单个断点记录的能力。

### Changed
- `TaskRepo` 接口新增 `DeleteResumeKey` 方法，`taskrepo_fake_test.go` 同步更新 mock 实现。

## [0.17.0] - 2026-08-01 11:02:53

### Added
- **虚拟滚动组件 `VirtualScroll.vue`**：替代原生 DOM 全量渲染，仅渲染可视区域内的媒体项，大幅降低 DOM 节点数与内存占用。
- **媒体智能预加载系统**：新增 `useMediaPreloader.ts`（前端预加载编排）与 `thumb_preloader.go`（后端缩略图批量预取），在用户浏览媒体列表时提前加载相邻缩略图，消除滚动白块。
- **智能图片加载器 `useSmartImageLoader.ts`**：基于 IntersectionObserver 的懒加载与视口优先级调度，进入视口才发起缩略图请求，减少首屏带宽竞争。
- **分层缓存系统 `tiered_cache.go`**：L1 内存 LRU + L2 磁盘缓存，支持命中率统计、容量管理，新增 `ChatService` Wails 绑定（`WarmupCache`/`GetCacheStats`/`GetCacheHitRate`）。
- **智能缓存预热 `smart_cache_warmer.go`**：根据用户浏览行为预测并预加载热点对话的缩略图到分层缓存，支持开关控制（`SetPreloadEnabled`/`IsPreloadEnabled`）。
- **性能监控与验证工具**：新增 `performance.ts`（前端性能计时、标记、指标上报）与 `performanceValidator.ts`（性能基准验证），后端 `ReportPerformance` 绑定接收前端指标并写入结构化日志。
- **性能基准测试工具**：新增 `src/cmd/performance-benchmark/` 命令行工具与 `performance_suite.go`/`performance_test.go` 后端性能测试套件。
- **数据库索引优化（schemaVersion 4→5）**：新增 5 个复合覆盖索引（`idx_files_task_state`/`idx_task_items_dialog`/`idx_files_dialog_covering` 等），优化大量数据下的媒体查询与任务文件查询性能。
- **性能优化文档**：新增 `doc/performance-optimization-summary.md`（优化摘要）与 `doc/performance-optimization-completion-report.md`（完成报告）。

### Changed
- `ChatsPage.vue` 大规模重构：集成 VirtualScroll 虚拟滚动、媒体预加载、智能图片加载、性能监控四套系统，244 行增量。
- `MediaPreview.vue` 集成媒体预加载器与性能监控，预览时自动预加载相邻媒体。
- `api.ts` 新增 11 个性能优化相关 Wails 绑定方法导出。

### Added
- **日志行数上限可配置**：新增 `maxLogLines` 配置项（`config.yaml` → `ui.maxLogLines`），将日志环形缓冲区容量从硬编码 5000 行改为用户可配置（16–1024，默认 128），设置页「界面」段 `InputNumber` 控制，即时生效。
- **日志行复制按钮**：每条日志行号沟槽内新增复制图标按钮，点击一键复制该行原始文本到剪贴板。

### Changed
- `config.Manager` 修复 `applyYAML` / `applyLegacyJSON` 中遗漏的 `UISettings.withDefaults()` 调用，确保 YAML/JSON 导入配置时 UI 字段也有零值回默认与钳制。

## [0.15.0] - 2026-08-01 12:38:00

### Added
- **对话页状态保持**：切到其他标签页再返回后，自动恢复上次选中的对话、已加载媒体列表、筛选条件与滚动位置；不自动重开 MediaPreview 浮层。实现依赖 `useMediaPager` 的 LRU 快照缓存（新增 `scrollTop` 字段）与 `localStorage` 持久化最后选中对话 ID。
- **日志页字号与滚动条自定义**：字号默认 14px（原 12px），支持 Ctrl+滚轮实时调整（10–28px 钳制），工具栏提供 `-` / `A` / `+` 三键组供无滚轮场景；滚动条尺寸（6–24px）由设置页「界面」段 `InputNumber` 控制，即时生效。两者均持久化到 `config.yaml` 的 `ui` 段。
- **日志导出对话框**：日志页工具栏新增「导出」按钮，打开 `ExportLogsDialog.vue`，支持选择范围（当前过滤结果 / 全部）、格式（`.log` / `.txt` / `.csv`）、目录（`DirSelect` 组件，kind=`logExport`）与文件名；`.csv` 按 RFC4180 转义含逗号/引号/换行的字段。
- **脚本启用开关**：脚本列表项左侧新增 Checkbox，勾选后该脚本出现在「添加下载」弹窗的脚本下拉中；未启用脚本不显示。启用状态持久化到 `config.yaml` 的 `scripts.enabled` 段，重启后保持。
- **内置脚本模板首次播种**：应用首次启动时自动将 4 个内置模板（`rename-by-date`、`filter-media`、`skip-duplicates`、`auto-archive`）写入脚本目录，默认全部禁用；用户删除后不再自动恢复，可从「模板」弹窗重新生成。播种标记 `templatesSeeded` 持久化到 config.yaml。
- **目录历史组件 `DirSelect.vue`**：统一替换设置页与「添加下载」弹窗中的手动目录输入，支持最近 3 条历史下拉（按 `download`/`logExport`/`cache`/`temp`/`logDir` 分组）、浏览按钮与手工输入目录的自动入库。
- **设置页「界面」段**：新增日志字体大小（10–28）与滚动条尺寸（6–24）两个 `InputNumber`，hint 注明 Ctrl+滚轮亦可调整字号。

### Changed
- `ScriptService.List()` 现在按 `config.IsScriptEnabled()` 填充每个脚本的 `enabled` 字段；`Delete()` 成功后调用 `cfg.ForgetScript()` 清理启用记录。
- `DownloadService.SelectDirectory` 默认目录优先级改为 `currentDir` → `cfg.RecentDirs(kind)[0]` → `cfg.Get().DownloadDir`；用户选定后自动 `cfg.AddRecentDir(kind, picked)`。
- `SettingsService.RecentDirs` / `AddRecentDir` 上限由 10 条降至 3 条，委托 `config.Manager` 统一管理。
- `config.Manager` 新增 `UISettings`（`logFontSize`/`scrollbarSize`）、`EnabledScripts`、`TemplatesSeeded` 字段；`Update()` 内新增 UI 零值回默认与上下限钳制、RecentDirs 裁剪至 3 条。

## [0.14.0] - 2026-08-01 01:26:14

### Added
- **日志导出功能**：日志页新增「导出」按钮与导出对话框（`ExportLogsDialog.vue`），支持选择导出范围（当前筛选结果 / 全部记录）与文件格式（`.log` 原始文本 / `.csv` 结构化表格含 time/level/module/source/message 列），后端 `LogService.ExportLogs` 执行文件写出；新增 `logsvc_export_test.go` 单元测试。
- **脚本内置模板系统**：新增 `src/internal/script/templates/` 嵌入 4 个内置脚本模板（自动归档 `auto-archive`、媒体过滤 `filter-media`、按日期重命名 `rename-by-date`、跳过重复 `skip-duplicates`），通过 `go:embed` 嵌入二进制，`TemplateService` 提供清单 API；前端脚本编辑器新增「模板」按钮与 `ScriptTemplateDialog.vue` 弹窗，选择模板后灌入编辑器可修改保存；新增 `templates_test.go` 单元测试。
- **DirSelect 可复用目录选择器**：新增 `DirSelect.vue` 组件（PrimeVue Select + 文件夹浏览按钮），支持历史下拉与自动填充最近目录，按 `kind` 分组持久化历史，统一各处手动目录选择实现。
- **RepoWiki 知识库文档**：新增下载引擎状态机、Wails 服务层 API 契约、前端页面架构、脚本引擎安全约束、主题切换系统、事件驱动架构等架构文档及对应知识卡。

## [0.13.1] - 2026-07-31 22:54:03

### Fixed
- **第二轮全库审计 13 项修复**（`doc/audit/20260731-2021-second-round-audit/`，Medium 6 / Low 7 全部闭环）：
  - ARC-001：ChatService 生命周期句柄（cancel/dead）在建连 goroutine 启动前发布，StopAndWait 覆盖"建连中"盲区。
  - ARC-002：store 增加 closed 标志与 `ErrStoreClosed`，engine 超时逃逸 goroutine 写入已关闭存储时降级为日志告警，不再 panic。
  - LOGIC-001：auth 登录流程增加 `flowDone` join——Logout/重新登录先 `cancelLoginAndWait(3s)` 等待旧流程 goroutine 真正退出，杜绝幽灵会话回写。
  - LOGIC-002：thumbCache 引入 epoch 代际，Clear 时代际自增，锁外 fetch 完成后入锁校验代际再落盘，消除清空后幽灵文件与 Windows 下 Clear 失败。
  - CODE-001：脚本超时熔断统一出口 `fuseContract`（Error 级宿主日志 + 前端脚本日志推送）；yaegi 执行体不可中断为已知限制，已固化至脚本调试教程。
  - FUNC-001：ChatsPage 单/双击延时定时器抽取 `resetThumbClick()`，切换对话/登出/卸载三处复位，不再误开旧对话预览。
  - LOGIC-003：登录态双源对账——启动时 kv 会话为权威源校准 config 展示态（`ReconcileOnStartup`）；`LoginStatus` 新增 `sessionPresent`，登录页对失步状态给出自愈提示。
  - CODE-003：`writeFileAtomic` rename 失败判据从"目标已存在"收窄为错误类型（`fs.ErrExist` / Windows 共享冲突 32 / 拒绝访问 5）。
  - CODE-002：LogsPage 搜索防抖定时器补 `onBeforeUnmount` 清理。
  - FUNC-002：MediaPreview `pendingAdvance` 在 items 收缩或首元素身份变化（列表整体重置）时复位，保留末项续拉自动前进主路径。

### Changed
- ARC-003：缩略图/视频队列 worker 数与下载进度事件节流间隔配置化（config.yaml `tuning` 段：`thumbWorkers`/`videoWorkers`/`progressIntervalMs`，零值回退内置默认 2/2/200ms，无 UI 入口）。
- ARC-004：engine 对持久化依赖接口化——新增 `TaskRepo` 窄接口（18 方法），`Deps.Store` 由 `*store.Store` 改为接口类型，装配零改动；新增内存 fake 单测证明可测试性。
- CODE-004：前端路由页面懒加载（LoginPage 保留静态导入保首屏）+ `@primeuix` 主题引擎拆独立 vendor chunk，构建产物由单 chunk 1.1MB 降为多 chunk（最大 332kB），500kB 警告消除。

## [0.13.0] - 2026-07-31 00:36:44

### Added
- **使用教程页**：新增「教程」标签页（`/tutorial`），按 基础 → 中级 → 高级 三级组织 8 个章节（安装启动、账号登录、创建下载任务、代理与设置、脚本语法基础、脚本实战示例、内置函数参考、脚本调试）；左侧章节导航含分级分组与当前章节 h2/h3 小节目录（锚点平滑跳转），底部提供上一章/下一章切换。
- **Markdown 渲染组件**：新增 `MarkdownView.vue` + `utils/markdown.ts`（markdown-it `html:false` 防注入 + highlight.js 按需注册 go/bash/yaml/json），样式全部使用 `--p-*` 变量适配明暗主题；新增依赖 `markdown-it`、`highlight.js`、`@types/markdown-it`。
- **代码块一键复制与行号**：所有围栏代码块带工具栏（语言标签 + 复制按钮，复制成功显示"已复制 ✓"，剪贴板 API 失败回退 execCommand）；Go 脚本示例代码块附带行号列（sticky 定位，选中/复制不含行号）。
- **教程内嵌真实图标**：`{{icon:xxx}}` 白名单占位符机制，界面总览表格等处直接渲染与左侧导航同套 `nav-*.svg` 图标（currentColor 随主题变色），替换原先与实际 UI 不符的 emoji。
- **关于页**：新增「关于」标签页（`/about`），居中英雄区（logo/名称/版本徽章/简介/操作按钮）+ 技术栈网格 + 作者与许可证元信息卡片；版本号经 Vite `define` 注入 `__APP_VERSION__`（读取 package.json，消除硬编码）；外链经 `BrowserOpenURL` 打开系统浏览器。
- **导航图标**：新增 `nav-tutorial.svg`（打开的书本）与 `nav-about.svg`（信息圆圈），注册至 `AppIcon.vue` 并同步到 `img/icons/navigation/`。

### Fixed
- 修复暗色主题下行内代码背景色因选择器特异性渗透到代码块内部导致缩进处出现浅色条的问题。

## [0.12.0] - 2026-07-30 23:39:21

### Added
- **自定义视频播放器**：新建 `VideoPlayer.vue` 组件，移除原生 `<video controls>`，基于 PrimeVue Button/Slider 自建控制栏（播放/暂停、可拖拽进度条含 buffered 区段显示、时间显示、音量滑条与静音、全屏切换），播放中鼠标静止 3s 自动隐藏控制栏，暂停时常显。
- **播放器键盘快捷键**：视频条目预览时 ←/→ 快退/快进 10 秒、↑/↓ 音量 ±10%、Space 播放暂停、M 静音、F 全屏；非视频条目保持 ←/→ 切换媒体（`utils/format.ts` 新增 `fmtDuration`）。
- **视频磁盘分段缓存（边下边播）**：新增 `videodisk.go`，在线视频分段落盘 `<TempDir>/video/<dialogID>_<msgID>/<partIdx>.part`，2GB 上限按 mtime LRU 淘汰；缓存查找链升级为 内存 LRU → 磁盘 → Telegram（`video.go`），重看/回拖命中磁盘不重复拉取。
- **后台顺序预取**：`videoPrefetcher` 单活跃视频从当前播放位置顺序预取全部分段；新增 `StopVideoPrefetch` 绑定，关闭预览/切换条目时停止预取省流量。
- **视频临时目录设置**：`Settings.TempDir` 新配置项（config.yaml + 设置页，含浏览按钮与可写校验），留空回落 `<数据目录>/tmp`；「清空缓存」联动清理视频临时分段。

### Fixed
- 修复暂停后 seek 被 `canplay` 事件强制恢复播放的问题（自动播放仅首次 canplay 触发）。
- 修复点击视频预览四周空白区域无法关闭预览的问题（`.mp-video-wrap` 铺满舞台吞掉了 mousedown，改为 wrap 自身被点击时关闭）。

## [0.11.0] - 2026-07-30 21:30:05

### Added
- **重复下载确认弹窗**：当用户尝试下载一个已处于下载中状态的多媒体文件时，弹出确认对话框，告知"该文件当前正在下载中，是否确认重复下载？"，提供"取消"/"重复下载"两个选项。
  - 前端 `ChatsPage` 新增 `isDownloading(messageId)` 检测函数：以 `fileStates`（`task:file` 事件实时 Map）为主、`tasksStore.files`（Pinia）为兜底，保证切页/刷新后仍可检测。
  - `downloadOne()` 优先级调整：下载中 → 弹"重复下载"框；已下载 → 弹"重新下载"框；未下载 → 直接打开添加下载对话框。
  - `downloadSelected()` 批量路径：统计下载中数与已下载数，生成合并文案"选中文件中有 N 个正在下载中、M 个已下载，是否确认重复下载？"。
  - 修复 `wails.json` 中 `productVersion` 字段缺失逗号的语法错误。

## [0.10.0] - 2026-07-30 20:26:52

### Added
- **对话媒体下载状态增强**：已下载标记 + 重复下载确认框 + 双击系统默认程序打开 + 下载中实时进度角标（含跨页面状态恢复）
  - 后端：store 层新增 `idx_files_dialog_msg` 索引与 `ListDoneFilesByDialog`/`GetDoneFile` 查询；engine 层 `TaskFile` 补 `DialogID`/`MessageID` 并写库（修复原有关联缺口），`FileEvent` 携带最终路径；services 层新增 `ListDownloadedMessages`（os.Stat 过滤磁盘已删文件 + 同 messageId 去重）与 `OpenDownloadedFile`
  - 前端：`ChatsPage` 新增 `downloadedMap` 与已下载 chip、下载中角标文案改为"下载中 N%"、重复下载 ConfirmDialog（单文件/批量）、双击打开（缩略图区 250ms 延时区分单击/双击）、切换对话/路由时状态恢复（tasks store 回填 downloading 角标）
  - 前端：`MediaPreview` 新增 `downloadedIds` prop 与 `open` 事件，顶栏显示"已下载"Tag 与"打开文件"按钮
- 数据库迁移：schemaVersion 3→4，新增 `idx_files_dialog_msg` 索引

### Fixed
- 修复 engine 层写 files 表时未携带 `dialog_id`/`message_id` 的既有缺口，仅新下载记录具备对话关联。

## [0.8.0] - 2026-07-29 21:14:08

### Added
- **品牌视觉素材系统**：全套图标、Logo、宣传海报、空状态插图接入项目
  - 新增 `img/` 目录（Logo、导航图标、登录图标、状态图标、插图、宣传材料），含完整 README 规范文档
  - 新增 `AppIcon.vue` 组件：内联 11 个品牌 SVG 图标（currentColor 描边，自动适配明暗主题）
  - 侧边栏品牌位由文本 "tdl" 替换为品牌图标图片
  - 导航图标（对话/下载/脚本/日志/设置）、账号、主题切换全部接入品牌图标
  - 登录页顶部新增英雄插图，三个登录方式 Tab 图标替换为品牌图标（验证码/二维码/Desktop）
  - TasksPage 与 ChatsPage 空状态替换为专用插图（empty-tasks / empty-chats）
  - README.md / README_EN.md 顶部接入横幅头图，界面预览节加入宣传海报
- **Wails 构建图标**：替换 `src/build/appicon.png` 与 `src/build/windows/icon.ico`
- **前端 favicon**：`index.html` 新增 SVG + PNG 双格式 favicon
- **类型支持**：新增 `vite-env.d.ts` 提供 `?raw` SVG 导入类型声明

## [0.7.0] - 2026-07-29 20:50:59

### Removed
- **任务追加下载功能整条链路已撤回。** 此前误将 tdl CLI 的单任务限制套用到 GUI，实际上 GUI 的 `Manager.Create` 天然支持并发独立任务（无互斥，每任务独立 client + dcpool），无需追加功能。
  - 前端：移除下载页终态任务卡片上的「追加下载」加号按钮、`NewTaskDialog` 追加模式（`appendMode`/`taskId` props、追加提示块、条件渲染的三块表单）、`tasks.ts` 的 `appendTask` action、`api.ts` 的 `appendTaskItems` 封装、`types.ts` 的 `AppendOptions` 接口。
  - 后端：移除 `services.DownloadService.AppendTaskItems`、`engine.AppendOptions` / `Manager.AppendItems` / `Task.mergeOpts` / `unionIntSlice`、`store.AppendURLItem` / `MergeSelectionItem` / `unionInts` 及对应单测。
  - SQLite 表结构（`task_items`、`uq_items_url` 索引）与迁移语句保持不动；创建任务时写入消息项、重启恢复时重组装仍依赖它。

## [0.6.0] - 2026-07-29 02:01:16

### Added
- SQLite 持久化层（`internal/store`）：`tasks`/`task_items`/`files`/`resume_points` 四表，支持 WAL 模式、手写 PRAGMA user_version 迁移、旧 `tasks.json` 单事务导入并生成 `.bak`。
- 任务追加下载：已完成/暂停/失败/取消态任务可追加新消息链接（URLs）或选集（Selections），采用「重启式」实现（running 走 Pause→runDone→Resume，done 置 paused）。
- YAML 配置迁移：`settings.json` → `config.yaml`，旧文件重命名为 `.bak`；`Settings` 保留 JSON 标签，YAML 用内部 `yamlConfig` 互转。
- 可配置缓存目录：`Settings` 新增 `cacheDir`，空值默认 `<DataDir>/cache`；保存时校验目录可写；`thumbCache` 改为 `rootFn` 注入实现热生效。
- 设置页新增「存储」区块：数据目录展示、缓存目录输入+浏览、清空缓存按钮（二次确认）。
- 断点续传改造：从 BBolt KV 的「逻辑位置」改为 SQLite `resume_points` 表的「内容坐标 `dialogID:messageID`」；分组消息逐成员生效。

### Changed
- 引擎层 `Manager` 从 JSON 全量 `persist()` 改为 SQLite 行级写（`UpdateTaskStatus`/`UpdateTaskCounts`/`UpsertFile`/`FinishFile` 等）。
- `execute()` 每轮从 `task_items` 表重组装 URLs/Selections，覆盖 queued 窗口期追加。
- 删除 `engine/persist.go` / `persist_test.go`，断点逻辑迁入 `resume_repo.go`。

## [0.5.1] - 2026-07-29 00:56:53

### Fixed
- 修复 ChatsPage 媒体过滤工具栏 UI 组件重叠问题。
  - 约束 `InputNumber` / `DatePicker` 内部 input 宽度跟随包装器，消除溢出遮挡相邻的「查询」「重置」按钮。
  - 为固定宽度输入框、按钮、分隔符增加 `flex-shrink: 0` 保护，防止容器宽度紧张时被挤压。
  - `spacer-flex` 增加 `min-width: 0`，保证换行后右侧布局切换 / 月份选择器正常靠右。
- 加固 `MediaPreview.vue` 顶栏弹性布局：文件名优先收缩省略，下载/关闭按钮不被压缩。

## [0.5.0] - 2026-07-29 00:35:07

### Added
- 新增完整日志记录与浏览功能。
- 后端日志核心（`internal/logging`）：基于 zap + lumberjack，支持多目标输出（文件+UI）、格式模板、文件滚动（MaxSize/MaxAge/MaxBackups）、内存环形缓冲（5000 条）与 250ms 批量事件推送。
- 后端全面埋点：app/auth/chat/download/engine/script/settings/media 等模块公开方法入口与错误分支。
- 新增 `LogService` Wails 绑定：实时日志快照、历史日志文件列表与读取、YAML 配置导入/导出、打开日志目录。
- 前端新增「日志」标签页（`/logs`）：VirtualScroller 类终端视图、行号沟槽、语法高亮、正则/大小写/自动跳转搜索、级别过滤、跟随滚动、历史文件浏览。
- 设置页新增「日志」配置区：输出目标、日志级别、目录、格式模板、滚动策略，支持 YAML 导入/导出。
- 数据目录下 `logging.yaml` 启动时自动加载；设置保存即热生效。
## [0.4.0] - 2026-07-28 23:36:21

### Added
- 下载任务记录持久化到磁盘（`tasks.json`），重启后保留；任务内文件级跟踪（.tmp → 最终路径 → done/failed）。
- 下载管理页新增：清除已完成记录、删除所有文件（ConfirmDialog 二次确认）、打开目录、文件列表展开与多选删除。
- 媒体列表支持瀑布流 / 网格 / 时间流三种布局切换，选择持久化到 localStorage。
- 时间流布局：按月份分组、右侧月份索引条锚点跳转、DatePicker 任意月份跳转（OffsetDate 查询管线）。
- Lightbox 全屏预览：图片大图缩放（0.5x–5x）/ 平移 / 双击复位，左右键盘切换，尾部自动 loadMore；视频仅预览大缩略图。
- 缩略图磁盘缓存（`cache/thumbs/` + `cache/previews/`）+ 本地 HTTP 服务（`/media/thumb`、`/media/preview`），响应头 `Cache-Control: immutable`。
- 新增 `MediaItem.Width/Height` 支持，瀑布流按纵横比占位。

### Changed
- 缩略图通道整体替换为 HTTP 服务（旧 data URI 通道移除，禁止向后兼容），浏览器接管并发与缓存。
- 下载任务 `TaskView` 新增 `FileCount` 字段；`Task` 增加 `files` 与 `done channel` 支持。
- `thumb.go` 超时提升至 3 分钟，适配单 worker 串行队列。

### Fixed
- 修复 `wails dev` 下 `explorer` 不在 PATH 导致「打开目录」失败：改用 `%SystemRoot%\explorer.exe` 绝对路径。
- 修复 Vite SPA fallback 吞 `/media/*` 请求：新增 `media404` 插件让 Wails 回落后端 Handler。

## [0.3.0] - 2026-07-28 21:18:01

### Added
- 对话页支持选集下载：勾选媒体后按对话 + 消息 ID 直接创建任务，不再依赖 t.me 链接，私聊/普通群消息也可下载。
- 媒体列表新增缩略图：内嵌模糊占位图（stripped thumb）即时展示，清晰缩略图按需拉取并内存缓存（ChatService.GetThumbnail）。
- 下载任务支持自定义标签（label），任务列表优先展示标签而非链接。

### Changed
- ChatDetailPage 合并入 ChatsPage，对话列表与媒体浏览统一为单页交互。
- 对话类型常量与 peer 解析（ResolveDialogPeer）下沉至 engine 层，供 services 与下载引擎复用。

## [0.2.0] - 2026-07-28 20:46:24

### Added
- 设置页新增「账号」区块，显示登录状态并提供账号管理/前往登录入口。
- 对话页新增加载失败错误提示与重试按钮，空态区分加载中/失败/无数据。
- 新增 ChatService 并发与错误路径回归测试（死锁、连接超时、Stop 复位）。

### Changed
- ChatService 连接启动重构：可注入 runClient、连接超时保护、ready/dead 竞态处理。
- 页面宽度统一为全宽布局，移除脚本页独立宽度限制。
- Desktop 会话导入成功提示补充双端会话冲突风险说明，导入按钮增加 loading 态。

### Fixed
- 修复 Desktop 会话导入未校验有效性导致的假登录态：导入后建连校验，失败自动回滚会话。

## [0.1.0] - 2026-07-28

### Added
- 初始化 tdl UI 项目，集成 tdl 核心库作为子模块。
- 添加 Wails 桌面应用框架基础结构。
- 添加前端 Vue 3 + PrimeVue 基础架构。
- 添加项目文档与基础配置文件。


