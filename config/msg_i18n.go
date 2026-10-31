package config

// 기동 단계(DB 연결 이전)의 운영자 대면 메시지 번역. 이 시점에는 settings 테이블을 읽을 수
// 없으므로 런타임 설정(ActiveLanguage)이 아니라 구성된 기본 언어(Language(): ARTEX_LANG >
// 설정 파일 > en)를 기준으로 치환한다. 조치에 필요한 식별자(ARTEX_PG_DSN·database·dsn·
// host/user/dbname 등)는 번역하지 않고 원문을 그대로 둔다. "ko" 는 키 자체이므로 저장하지
// 않고, 조회 실패·ko 일 때는 원문을 돌려준다.
var cfgCatalog = map[string]map[string]string{
	"환경 변수 ARTEX_PG_DSN": {
		"en": "environment variable ARTEX_PG_DSN",
		"zh": "环境变量 ARTEX_PG_DSN",
		"es": "variable de entorno ARTEX_PG_DSN", "ja": "環境変数 ARTEX_PG_DSN",
	},
	"설정 파일 %s (database.dsn)": {
		"en": "config file %s (database.dsn)",
		"zh": "配置文件 %s (database.dsn)",
		"es": "archivo de configuración %s (database.dsn)", "ja": "設定ファイル %s (database.dsn)",
	},
	"설정 파일 %s (database 필드)": {
		"en": "config file %s (database fields)",
		"zh": "配置文件 %s (database 字段)",
		"es": "archivo de configuración %s (campos database)", "ja": "設定ファイル %s (database フィールド)",
	},
	"데이터베이스 설정을 찾을 수 없습니다: 환경 변수 ARTEX_PG_DSN 이 설정되어 있지 않고, 설정 파일 %s 에도 database (dsn 또는 host/user/dbname) 설정이 없습니다. 설정 파일을 만들거나 환경 변수를 설정한 뒤 다시 시도하세요": {
		"en": "database configuration not found: environment variable ARTEX_PG_DSN is not set, and config file %s has no database (dsn or host/user/dbname) configuration. Create a config file or set the environment variable, then try again",
		"zh": "找不到数据库配置：环境变量 ARTEX_PG_DSN 未设置，配置文件 %s 中也没有 database（dsn 或 host/user/dbname）配置。请创建配置文件或设置环境变量后重试",
		"es": "no se encontró la configuración de la base de datos: la variable de entorno ARTEX_PG_DSN no está definida y el archivo de configuración %s tampoco tiene configuración database (dsn o host/user/dbname). Cree un archivo de configuración o defina la variable de entorno e inténtelo de nuevo", "ja": "データベース設定が見つかりません: 環境変数 ARTEX_PG_DSN が設定されておらず、設定ファイル %s にも database (dsn または host/user/dbname) の設定がありません。設定ファイルを作成するか環境変数を設定してから、もう一度お試しください",
	},
}

// logCatalog holds operator-facing console log templates (startup + runtime
// diagnostics). Keyed by the Korean source template; format verbs are preserved in
// the same positional order so fmt.Printf/log.Printf arguments still line up.
var logCatalog = map[string]map[string]string{
	"[config] 설정 파일: %s": {
		"en": "[config] config file: %s", "zh": "[config] 配置文件: %s", "es": "[config] archivo de configuración: %s", "ja": "[config] 設定ファイル: %s",
	},
	"[config] 설정 파일: %s (파일 없음 · 환경 변수 ARTEX_PG_DSN 만 사용)": {
		"en": "[config] config file: %s (not found · using only environment variable ARTEX_PG_DSN)",
		"zh": "[config] 配置文件: %s (文件不存在 · 仅使用环境变量 ARTEX_PG_DSN)",
		"es": "[config] archivo de configuración: %s (no encontrado · usando solo la variable de entorno ARTEX_PG_DSN)", "ja": "[config] 設定ファイル: %s (ファイルなし · 環境変数 ARTEX_PG_DSN のみ使用)",
	},
	"[config] skill 디렉터리: %s": {
		"en": "[config] skill directory: %s", "zh": "[config] skill 目录: %s", "es": "[config] directorio de skills: %s", "ja": "[config] スキルディレクトリ: %s",
	},
	"일시정지됨": {"en": "paused", "zh": "已暂停", "es": "pausada", "ja": "一時停止しました"},
	"재개됨":   {"en": "resumed", "zh": "已恢复", "es": "reanudada", "ja": "再開しました"},
	"[auth] JWT 키를 %s 에서 %s 로 이전(탐색 가능한 워크스페이스 밖으로 이동)": {
		"en": "[auth] moved JWT key from %s to %s (out of the discoverable workspace)",
		"zh": "[auth] 已将 JWT 密钥从 %s 迁移到 %s（移出可被发现的工作区）",
		"es": "[auth] clave JWT movida de %s a %s (fuera del espacio de trabajo detectable)", "ja": "[auth] JWT キーを %s から %s へ移動しました(探索可能なワークスペースの外へ移動)",
	},
	"[auth] 새 JWT 키를 %s 에 기록": {
		"en": "[auth] wrote new JWT key to %s", "zh": "[auth] 已将新的 JWT 密钥写入 %s", "es": "[auth] nueva clave JWT escrita en %s", "ja": "[auth] 新しい JWT キーを %s に書き込みました",
	},
	"[auth] %s 가 %d자 미만이라 무시합니다. 설정 토큰 방식으로 초기화하세요": {
		"en": "[auth] ignoring %s because it is shorter than %d characters; initialise with the setup token instead",
		"zh": "[auth] %s 少于 %d 个字符，已忽略。请使用设置令牌方式初始化",
		"es": "[auth] se ignora %s porque tiene menos de %d caracteres; inicialice con el token de configuración", "ja": "[auth] %s が %d 文字未満のため無視します。設定トークン方式で初期化してください",
	},
	"[auth] %s 가 %d바이트를 넘어 무시합니다. 설정 토큰 방식으로 초기화하세요": {
		"en": "[auth] ignoring %s because it exceeds %d bytes; initialise with the setup token instead",
		"zh": "[auth] %s 超过 %d 字节，已忽略。请使用设置令牌方式初始化",
		"es": "[auth] se ignora %s porque supera los %d bytes; inicialice con el token de configuración", "ja": "[auth] %s が %d バイトを超えるため無視します。設定トークン方式で初期化してください",
	},
	"[auth] 비밀번호 상태를 읽지 못해 첫 실행 초기화를 건너뜁니다: %v": {
		"en": "[auth] could not read the password state; skipping first-run initialisation: %v",
		"zh": "[auth] 无法读取密码状态，跳过首次运行初始化: %v",
		"es": "[auth] no se pudo leer el estado de la contraseña; se omite la inicialización inicial: %v", "ja": "[auth] パスワードの状態を読み取れず、初回起動時の初期化をスキップします: %v",
	},
	"[auth] %s 해시 생성 실패: %v": {
		"en": "[auth] failed to hash %s: %v",
		"zh": "[auth] 生成 %s 哈希失败: %v",
		"es": "[auth] no se pudo generar el hash de %s: %v", "ja": "[auth] %s のハッシュ生成に失敗しました: %v",
	},
	"[auth] %s 저장 실패: %v": {
		"en": "[auth] failed to store %s: %v",
		"zh": "[auth] 保存 %s 失败: %v",
		"es": "[auth] no se pudo guardar %s: %v", "ja": "[auth] %s の保存に失敗しました: %v",
	},
	"[auth] 환경 변수 %s 로 관리자 비밀번호를 초기화했습니다(사용자 이름 ARTEX)": {
		"en": "[auth] initialised the admin password from environment variable %s (username ARTEX)",
		"zh": "[auth] 已通过环境变量 %s 初始化管理员密码（用户名 ARTEX）",
		"es": "[auth] contraseña de administrador inicializada desde la variable de entorno %s (usuario ARTEX)", "ja": "[auth] 環境変数 %s で管理者パスワードを初期化しました(ユーザー名 ARTEX)",
	},
	"[auth] 관리자 비밀번호가 아직 설정되지 않았습니다. 설정 토큰은 환경 변수 %s 값입니다(로그에 출력하지 않음)": {
		"en": "[auth] the admin password has not been set yet. The setup token is the value of environment variable %s (not printed to the log)",
		"zh": "[auth] 管理员密码尚未设置。设置令牌为环境变量 %s 的值（不输出到日志）",
		"es": "[auth] aún no se ha establecido la contraseña de administrador. El token de configuración es el valor de la variable de entorno %s (no se muestra en el registro)", "ja": "[auth] 管理者パスワードがまだ設定されていません。設定トークンは環境変数 %s の値です(ログには出力しません)",
	},
	"[auth] 설정 토큰 생성 실패: %v": {
		"en": "[auth] failed to generate the setup token: %v",
		"zh": "[auth] 生成设置令牌失败: %v",
		"es": "[auth] no se pudo generar el token de configuración: %v", "ja": "[auth] 設定トークンの生成に失敗しました: %v",
	},
	"[auth] 관리자 비밀번호가 아직 설정되지 않았습니다. 설정 토큰: %s": {
		"en": "[auth] the admin password has not been set yet. Setup token: %s",
		"zh": "[auth] 管理员密码尚未设置。设置令牌: %s",
		"es": "[auth] aún no se ha establecido la contraseña de administrador. Token de configuración: %s", "ja": "[auth] 管理者パスワードがまだ設定されていません。設定トークン: %s",
	},
	"[auth] 첫 화면(/setup)의 \"설정 토큰\" 칸에 위 값을 입력하거나 POST /api/auth/init 본문의 setup_token 으로 보내십시오. 이 토큰은 네트워크로 전달되지 않으며 이 콘솔 로그에서만 확인할 수 있습니다": {
		"en": "[auth] enter the value above in the \"Setup token\" field on the first screen (/setup), or send it as setup_token in the POST /api/auth/init body. This token is never sent over the network and is only visible in this console log",
		"zh": "[auth] 请在首个页面（/setup）的“设置令牌”栏中输入上述值，或将其作为 POST /api/auth/init 请求体中的 setup_token 发送。该令牌不会通过网络传输，只能在此控制台日志中查看",
		"es": "[auth] introduzca el valor anterior en el campo \"Token de configuración\" de la primera pantalla (/setup) o envíelo como setup_token en el cuerpo de POST /api/auth/init. Este token nunca se transmite por la red y solo puede verse en este registro de consola", "ja": "[auth] 最初の画面(/setup)の\"設定トークン\"欄に上記の値を入力するか、POST /api/auth/init の本文に setup_token として送信してください。このトークンはネットワークでは送信されず、このコンソールログでのみ確認できます",
	},
	"[auth] 경고: 관리자 비밀번호가 아직 설정되지 않았는데 서버가 %q 에 바인딩되어 다른 호스트에서도 접속할 수 있습니다. 설정 토큰 없이는 초기화할 수 없지만, 초기 설정을 마칠 때까지 이 포트를 외부 네트워크에 노출하지 마십시오(기본값 127.0.0.1:8787)": {
		"en": "[auth] warning: the admin password has not been set yet and the server is bound to %q, so other hosts can connect. Initialisation still requires the setup token, but do not expose this port to external networks until initial setup is complete (default 127.0.0.1:8787)",
		"zh": "[auth] 警告：管理员密码尚未设置，但服务器绑定在 %q，其他主机也可以连接。虽然没有设置令牌无法初始化，但在完成初始设置之前请勿将此端口暴露到外部网络（默认 127.0.0.1:8787）",
		"es": "[auth] advertencia: aún no se ha establecido la contraseña de administrador y el servidor está enlazado a %q, por lo que otros hosts pueden conectarse. La inicialización sigue requiriendo el token de configuración, pero no exponga este puerto a redes externas hasta completar la configuración inicial (predeterminado 127.0.0.1:8787)", "ja": "[auth] 警告: 管理者パスワードがまだ設定されていないのに、サーバーが %q にバインドされており、他のホストからも接続できます。設定トークンがなければ初期化できませんが、初期設定が完了するまでこのポートを外部ネットワークに公開しないでください(デフォルト 127.0.0.1:8787)",
	},
	"[auth] 초기 설정 거부: 설정 토큰 불일치 (client=%s)": {
		"en": "[auth] initial setup rejected: setup token mismatch (client=%s)",
		"zh": "[auth] 拒绝初始设置：设置令牌不匹配 (client=%s)",
		"es": "[auth] configuración inicial rechazada: el token de configuración no coincide (client=%s)", "ja": "[auth] 初期設定を拒否: 設定トークンが一致しません (client=%s)",
	},
	"[auth] 관리자 비밀번호를 초기화했습니다 (client=%s)": {
		"en": "[auth] admin password initialised (client=%s)",
		"zh": "[auth] 已初始化管理员密码 (client=%s)",
		"es": "[auth] contraseña de administrador inicializada (client=%s)", "ja": "[auth] 管理者パスワードを初期化しました (client=%s)",
	},
	"[auth] 비밀번호 변경 거부: 현재 비밀번호 불일치 (client=%s)": {
		"en": "[auth] password change rejected: current password mismatch (client=%s)",
		"zh": "[auth] 拒绝修改密码：当前密码不匹配 (client=%s)",
		"es": "[auth] cambio de contraseña rechazado: la contraseña actual no coincide (client=%s)", "ja": "[auth] パスワード変更を拒否: 現在のパスワードが一致しません (client=%s)",
	},
	"[auth] 관리자 비밀번호를 변경했습니다. 기존 세션은 모두 무효화됩니다 (client=%s)": {
		"en": "[auth] admin password changed; all existing sessions are invalidated (client=%s)",
		"zh": "[auth] 已修改管理员密码，所有现有会话均已失效 (client=%s)",
		"es": "[auth] contraseña de administrador cambiada; todas las sesiones existentes quedan invalidadas (client=%s)", "ja": "[auth] 管理者パスワードを変更しました。既存のセッションはすべて無効化されます (client=%s)",
	},
	"[auth] 로그인 실패 (client=%s)": {
		"en": "[auth] login failed (client=%s)",
		"zh": "[auth] 登录失败 (client=%s)",
		"es": "[auth] inicio de sesión fallido (client=%s)", "ja": "[auth] ログインに失敗しました (client=%s)",
	},
	"[notify] 백그라운드 알림 전송 루프가 %s 로 꺼졌습니다(테스트 전용) — 취약점 IM 알림이 전송되지 않습니다": {
		"en": "[notify] background notification loop disabled via %s (test only) — vulnerability IM alerts will not be sent",
		"zh": "[notify] 后台通知发送循环已通过 %s 关闭（仅测试）— 不会发送漏洞 IM 通知",
		"es": "[notify] el bucle de notificaciones en segundo plano se desactivó mediante %s (solo pruebas) — no se enviarán alertas IM de vulnerabilidades", "ja": "[notify] バックグラウンド通知送信ループが %s により無効化されています(テスト専用) — 脆弱性 IM 通知は送信されません",
	},
	"[mcp] %s 에서 도구 %d개를 발견해 캐시했습니다": {
		"en": "[mcp] from %s, discovered and cached %d tool(s)",
		"zh": "[mcp] 从 %s 发现并缓存了 %d 个工具",
		"es": "[mcp] desde %s, se descubrieron y almacenaron en caché %d herramienta(s)", "ja": "[mcp] %s からツールを %d 件検出し、キャッシュしました",
	},
	"[mcp] 시작 시 자동 발견: 목록을 읽지 못했습니다: %v": {
		"en": "[mcp] auto-discovery at startup: failed to read the list: %v",
		"zh": "[mcp] 启动时自动发现：读取列表失败：%v",
		"es": "[mcp] descubrimiento automático al inicio: no se pudo leer la lista: %v", "ja": "[mcp] 起動時の自動検出: 一覧を読み取れませんでした: %v",
	},
	"[mcp] 시작 시 %s 자동 발견에 실패했습니다: %v": {
		"en": "[mcp] auto-discovery of %s at startup failed: %v",
		"zh": "[mcp] 启动时自动发现 %s 失败：%v",
		"es": "[mcp] falló el descubrimiento automático de %s al inicio: %v", "ja": "[mcp] 起動時の %s の自動検出に失敗しました: %v",
	},
	"[tools] orchestration/platform 도구 스키마를 코드 기본값으로 새로고침(1회성)": {
		"en": "[tools] refreshed orchestration/platform tool schemas to code defaults (one-time)",
		"zh": "[tools] 已将 orchestration/platform 工具 schema 刷新为代码默认值（一次性）",
		"es": "[tools] esquemas de herramientas orchestration/platform actualizados a los valores predeterminados del código (una sola vez)", "ja": "[tools] orchestration/platform ツールのスキーマをコードのデフォルト値に更新しました(1回限り)",
	},
	"[reporter] 「%s」 agent + finding 트리거 사전 구성": {
		"en": "[reporter] pre-configured the \"%s\" agent + finding trigger",
		"zh": "[reporter] 已预配置「%s」agent + finding 触发器",
		"es": "[reporter] agente \"%s\" + disparador de finding preconfigurados", "ja": "[reporter] 「%s」 agent + finding トリガーを事前構成しました",
	},
	"[custom-tool] python 인터프리터 자동 감지: %s": {
		"en": "[custom-tool] auto-detected python interpreter: %s",
		"zh": "[custom-tool] 自动检测到 python 解释器：%s",
		"es": "[custom-tool] intérprete de python detectado automáticamente: %s", "ja": "[custom-tool] python インタープリターを自動検出: %s",
	},
	"[pg] 데이터베이스 설정 출처: %s": {
		"en": "[pg] database configuration source: %s", "zh": "[pg] 数据库配置来源：%s", "es": "[pg] origen de la configuración de la base de datos: %s", "ja": "[pg] データベース設定の取得元: %s",
	},
	"[mcp] browser MCP 캡처 프록시 연결: %s (CA %s)": {
		"en": "[mcp] browser MCP capture proxy attached: %s (CA %s)",
		"zh": "[mcp] browser MCP 抓包代理已连接：%s (CA %s)",
		"es": "[mcp] proxy de captura de browser MCP conectado: %s (CA %s)", "ja": "[mcp] browser MCP キャプチャプロキシを接続: %s (CA %s)",
	},
	"[mcp] browser MCP 캡처 프록시 설정 제거": {
		"en": "[mcp] browser MCP capture proxy configuration removed",
		"zh": "[mcp] 已移除 browser MCP 抓包代理配置",
		"es": "[mcp] configuración del proxy de captura de browser MCP eliminada", "ja": "[mcp] browser MCP キャプチャプロキシの設定を削除しました",
	},

	// Failover / planner / worker lifecycle log sites (llmpool, engine, task_llm).
	// These keys are the ENGLISH source templates that now live in the call sites
	// themselves; only the non-English translations are stored here. ru is
	// explicit; languages without an entry (es/ja and the unwired test default)
	// resolve to the key itself.
	"[llmpool] profile %q(%s) breaker tripped: %s": {
		"ru": "[llmpool] профиль %q(%s) аварийно отключён по сбою: %s",
		"zh": "[llmpool] 配置 %q(%s) 已熔断：%s",
	},
	"[llmpool] LLM failover: %q(%s) -> %q(%s), cause: %s": {
		"ru": "[llmpool] отказ LLM-профиля: %q(%s) -> %q(%s), причина: %s",
		"zh": "[llmpool] LLM 故障转移：%q(%s) → %q(%s)，原因：%s",
	},
	"[llmpool] failover chain exhausted (%d profiles all failed), last error: %s": {
		"ru": "[llmpool] цепь отказоустойчивости исчерпана (не прошёл ни один из %d профилей), последняя ошибка: %s",
		"zh": "[llmpool] 轮询链已耗尽(%d 个配置全部失败)，最后错误：%s",
	},
	"[llmpool] failed to persist breaker state: %v": {
		"ru": "[llmpool] не удалось сохранить состояние автомата: %v",
		"zh": "[llmpool] 熔断状态落库失败: %v",
	},
	"[llmpool] restored breaker state: profile #%d cooling until %s": {
		"ru": "[llmpool] восстановлено состояние автомата: профиль #%d остывает до %s",
		"zh": "[llmpool] 恢复熔断状态: 配置 #%d 冷却至 %s",
	},
	"[llmpool] failed to read failover chain: %v": {
		"ru": "[llmpool] не удалось прочитать цепь отказоустойчивости: %v",
		"zh": "[llmpool] 读取轮询链失败: %v",
	},
	"[llmpool] LLM failover enabled, chain(%d): %v": {
		"ru": "[llmpool] отказоустойчивость LLM включена, цепь(%d): %v",
		"zh": "[llmpool] LLM 轮询已启用，链路(%d): %v",
	},
	"[task-llm] task %s non-streaming call failed, retrying same provider in %v (%d/%d): %v": {
		"ru": "[task-llm] задача %s нестриминговый вызов не удался, повтор на том же provider через %v (%d/%d): %v",
		"zh": "[task-llm] task %s 非流式调用失败,%v 后同 provider 重试 (%d/%d): %v",
	},
	"[task-llm] task %s pre-commit stream failed, retrying same provider in %v (%d/%d): %v": {
		"ru": "[task-llm] задача %s сбой потока до передачи данных, повтор на том же provider через %v (%d/%d): %v",
		"zh": "[task-llm] task %s 提交前流失败,%v 后同 provider 重试 (%d/%d): %v",
	},
	"[planner] task %s planning… (%s trigger)": {
		"ru": "[planner] задача %s планирование… (триггер %s)",
		"zh": "[planner] task %s 规划中…(%s 触发)",
	},
	"[planner] task %s planning failed: %v": {
		"ru": "[planner] задача %s ошибка планирования: %v",
		"zh": "[planner] task %s 规划出错: %v",
	},
	"[planner] task %s goal met: %s": {
		"ru": "[planner] задача %s цель достигнута: %s",
		"zh": "[planner] task %s 判定目标达成: %s",
	},
	"[planner] task %s failed to persist completion: %v": {
		"ru": "[planner] задача %s не удалось сохранить статус завершения: %v",
		"zh": "[planner] task %s 标记完成落库失败: %v",
	},
	"[planner] task %s planning done": {
		"ru": "[planner] задача %s планирование завершено",
		"zh": "[planner] task %s 规划完成",
	},
	"[worker %s] task %s claimed intent #%d": {
		"ru": "[worker %s] задача %s взял в работу intent #%d",
		"zh": "[worker %s] task %s 领取意图 #%d",
	},
	"[goalless] task %s failed to settle as done: %v": {
		"ru": "[goalless] задача %s не удалось завершить со статусом done: %v",
		"zh": "[goalless] task %s 收尾落 done 失败: %v",
	},
	"[worker %s] task %s intent #%d ended with model_error, retrying in %v (%d/%d)": {
		"ru": "[worker %s] задача %s intent #%d завершён с model_error, повтор через %v (%d/%d)",
		"zh": "[worker %s] task %s 意图 #%d model_error 收场，%v 后重试 (%d/%d)",
	},
	"[worker %s] task %s intent #%d paused": {
		"ru": "[worker %s] задача %s intent #%d приостановлен",
		"zh": "[worker %s] task %s 意图 #%d 已暂停",
	},
	"[worker %s] task %s intent #%d stopped, waiting for cancel cleanup": {
		"ru": "[worker %s] задача %s intent #%d остановлен, ожидание очистки при отмене",
		"zh": "[worker %s] task %s 意图 #%d 已停止，等待取消清理",
	},
	"[worker %s] task %s intent #%d cancelled because the task is complete (stopped)": {
		"ru": "[worker %s] задача %s intent #%d отменён — задача уже завершена (stopped)",
		"zh": "[worker %s] task %s 意图 #%d 因任务已完成而取消(stopped)",
	},
	"[worker %s] task %s intent #%d terminated (stopped)": {
		"ru": "[worker %s] задача %s intent #%d досрочно остановлен (stopped)",
		"zh": "[worker %s] task %s 意图 #%d 被终止(stopped)",
	},
	"[worker %s] task %s intent #%d finished: %s (wrote back %s)": {
		"ru": "[worker %s] задача %s intent #%d завершён: %s (записано %s)",
		"zh": "[worker %s] task %s 意图 #%d 结束: %s (写回 %s)",
	},
	"[deadline] task %s timeout reached, starting settle sequence": {
		"ru": "[deadline] задача %s достигнут предел таймаута, запускается завершение",
		"zh": "[deadline] task %s 到达超时上限,进入收尾时序",
	},
	"[deadline] task %s drain timeout (%s), force-cancelling the running run": {
		"ru": "[deadline] задача %s истекло время drain (%s), жёсткая отмена текущего run",
		"zh": "[deadline] task %s drain 超时(%s),硬取消在跑 run",
	},
	"[deadline] task %s failed to persist terminal state: %v": {
		"ru": "[deadline] задача %s не удалось записать конечный статус: %v",
		"zh": "[deadline] task %s 落终态失败: %v",
	},
	"[deadline] task %s settle complete, terminal status=%s": {
		"ru": "[deadline] задача %s завершение выполнено, конечный статус=%s",
		"zh": "[deadline] task %s 收尾完成,终态=%s",
	},
	"[deadline] task %s already terminal during settle, keeping status": {
		"ru": "[deadline] задача %s на момент завершения статус уже конечный, оставлен без изменений",
		"zh": "[deadline] task %s 收尾时已是终态,保留原状态",
	},
	"[deadline] task %s settle planning failed: %v": {
		"ru": "[deadline] задача %s ошибка завершающего планирования: %v",
		"zh": "[deadline] task %s 终局规划出错: %v",
	},
	"[deadline] task %s settle: goal met: %s": {
		"ru": "[deadline] задача %s завершение: цель достигнута: %s",
		"zh": "[deadline] task %s 终局判定目标达成: %s",
	},
	"[deadline] task %s failed to stamp first_run: %v": {
		"ru": "[deadline] задача %s не удалось проставить метку first_run: %v",
		"zh": "[deadline] task %s 盖章 first_run 失败: %v",
	},
	"[deadline] task %s first run, deadline at %s": {
		"ru": "[deadline] задача %s первый запуск, дедлайн %s",
		"zh": "[deadline] task %s 首次运行,截止于 %s",
	},
	"[work %s] thinking-only turn (no text, no tool call) reached the continuation limit %d, letting the turn end": {
		"ru": "[work %s] пустой ход (только мышление, без текста и вызовов инструментов) достиг предела продолжения %d, ход завершается",
		"zh": "[work %s] 空转回合(仅思考、无正文无工具)已达续跑上限 %d，放行收场",
	},
	"[work %s] thinking-only turn (no text, no tool call), injecting continuation instruction (%d/%d)": {
		"ru": "[work %s] пустой ход (только мышление, без текста и вызовов инструментов), подставляется инструкция продолжения (%d/%d)",
		"zh": "[work %s] 空转回合(仅思考、无正文无工具)，注入续跑指令 (%d/%d)",
	},
	"[activity] task %s dropped activity record (record #%d for this task) worker=%s kind=%s tool=%s tuid=%s reason=%s summary=%q: %v%s": {
		"ru": "[activity] задача %s запись активности отброшена (запись #%d по задаче) worker=%s kind=%s tool=%s tuid=%s reason=%s summary=%q: %v%s",
		"zh": "[activity] task %s 丢弃活动记录(该任务累计第 %d 条) worker=%s kind=%s tool=%s tuid=%s reason=%s summary=%q: %v%s",
	},
	"[activity] task %s write succeeded on retry #%d (worker=%s kind=%s tool=%s)": {
		"ru": "[activity] задача %s запись успешна с %d-й попытки (worker=%s kind=%s tool=%s)",
		"zh": "[activity] task %s 写入第 %d 次重试成功 (worker=%s kind=%s tool=%s)",
	},
	"[activity] task %s write failed (attempt %d/3, worker=%s kind=%s tool=%s expID=%d): %v": {
		"ru": "[activity] задача %s ошибка записи (попытка %d/3, worker=%s kind=%s tool=%s expID=%d): %v",
		"zh": "[activity] task %s 写入失败 (第 %d/3 次, worker=%s kind=%s tool=%s expID=%d): %v",
	},
	"[worker %s] task %s intent #%d failed to roll back after claim: %v": {
		"ru": "[worker %s] задача %s intent #%d ошибка отката после взятия: %v",
		"zh": "[worker %s] task %s 意图 #%d 领取后回退失败: %v",
	},
	"[worker %s] task %s intent #%d failed to persist paused state: %v": {
		"ru": "[worker %s] задача %s intent #%d не удалось сохранить статус паузы: %v",
		"zh": "[worker %s] task %s 意图 #%d 暂停状态落库失败: %v",
	},
	"[worker %s] task %s intent #%d failed to persist cancel fence: %v": {
		"ru": "[worker %s] задача %s intent #%d не удалось сохранить барьер отмены: %v",
		"zh": "[worker %s] task %s 意图 #%d 取消栅栏落库失败: %v",
	},
	"[worker %s] task %s intent #%d failed to roll back for task pause: %v": {
		"ru": "[worker %s] задача %s intent #%d не удалось откатить при паузе задачи: %v",
		"zh": "[worker %s] task %s 意图 #%d 任务暂停回退失败: %v",
	},
	"[worker %s] task %s intent #%d failed to persist timeout-settle state: %v": {
		"ru": "[worker %s] задача %s intent #%d не удалось сохранить статус завершения по таймауту: %v",
		"zh": "[worker %s] task %s 意图 #%d 超时收尾状态落库失败: %v",
	},
	"[worker %s] task %s intent #%d ended by task-timeout settle (exhausted), wrote back %s": {
		"ru": "[worker %s] задача %s intent #%d завершён по таймауту задачи (exhausted), записано %s",
		"zh": "[worker %s] task %s 意图 #%d 因任务超时收尾结束(exhausted)，写回 %s",
	},
	"[worker %s] task %s intent #%d failed to persist stopped state: %v": {
		"ru": "[worker %s] задача %s intent #%d не удалось сохранить статус stopped: %v",
		"zh": "[worker %s] task %s 意图 #%d 终态停止落库失败: %v",
	},
	"[worker %s] task %s intent #%d failed to persist planner-stop state: %v": {
		"ru": "[worker %s] задача %s intent #%d не удалось сохранить статус остановки планировщиком: %v",
		"zh": "[worker %s] task %s 意图 #%d planner 停止落库失败: %v",
	},
	"[worker %s] intent %d hit the turn limit (exhausted), wrote back %s": {
		"ru": "[worker %s] intent %d достигнут предел шагов (exhausted), записано %s",
		"zh": "[worker %s] intent %d 撞步数上限(exhausted)，本次写回 %s",
	},
	"[worker %s] intent %d run timeout (exhausted), wrote back after settle %s": {
		"ru": "[worker %s] intent %d истекло время работы (exhausted), после завершения записано %s",
		"zh": "[worker %s] intent %d 运行超时(exhausted)，收尾后写回 %s",
	},
	"[worker %s] task %s intent #%d failed to persist terminal state %s: %v": {
		"ru": "[worker %s] задача %s intent #%d не удалось сохранить конечный статус %s: %v",
		"zh": "[worker %s] task %s 意图 #%d 终态 %s 落库失败: %v",
	},
	"[proxy] global proxy %q invalid, ignored: %v": {
		"ru": "[proxy] глобальный прокси %q некорректен, проигнорирован: %v",
		"zh": "[proxy] 全局代理 %q 无效，已忽略: %v",
	},
	"[mcp] browser proxy sync: failed to read MCP server list: %v": {
		"ru": "[mcp] синхронизация proxy браузера: не удалось прочитать список MCP: %v",
		"zh": "[mcp] browser 代理同步: 读取 MCP 列表失败: %v",
	},
	"[mcp] browser proxy sync failed: %v": {
		"ru": "[mcp] ошибка синхронизации proxy браузера: %v",
		"zh": "[mcp] browser 代理同步失败: %v",
	},
	"[engine] task %s reset %d stale running intents to open": {
		"ru": "[engine] задача %s сброшены %d зависших running intents в open",
		"zh": "[engine] task %s 重置 %d 个残留 running 意图为 open",
	},
	"[custom-tool] failed to load custom tool: %v": {
		"ru": "[custom-tool] не удалось загрузить пользовательский инструмент: %v",
		"zh": "[custom-tool] 加载失败: %v",
	},
	"[tools] failed to unbind goal_met from planner: %v": {
		"ru": "[tools] не удалось отвязать goal_met от planner: %v",
		"zh": "[tools] goal_met 解绑 planner 失败: %v",
	},
	"[prompts] failed to re-seed the goals prompt with the new default: %v": {
		"ru": "[prompts] не удалось перезаписать промпт goals новым дефолтом: %v",
		"zh": "[prompts] goals 提示词重刷为新默认失败: %v",
	},
	"[prompts] appended new default version to the goals prompt (adds an operation-constraints step, one-off)": {
		"ru": "[prompts] к промпту goals добавлена новая дефолтная версия (добавлен шаг операционных ограничений, разовая миграция)",
		"zh": "[prompts] goals 提示词已追加新默认版本(加入抽操作约束步,一次性)",
	},
	"[prompts] failed to re-seed the mainagent prompt with the new default: %v": {
		"ru": "[prompts] не удалось перезаписать промпт mainagent новым дефолтом: %v",
		"zh": "[prompts] mainagent 提示词重刷为新默认失败: %v",
	},
	"[prompts] appended new default version to the mainagent prompt (asks for new goals after a goal is met, one-off)": {
		"ru": "[prompts] к промпту mainagent добавлена новая дефолтная версия (запрашивает новые цели после достижения цели, разовая миграция)",
		"zh": "[prompts] mainagent 提示词已追加新默认版本(加入目标达成后反问建目标,一次性)",
	},
	"[prompts] failed to re-seed the planner prompt with the new default: %v": {
		"ru": "[prompts] не удалось перезаписать промпт planner новым дефолтом: %v",
		"zh": "[prompts] planner 提示词重刷为新默认失败: %v",
	},
	"[prompts] appended new default version to the planner prompt (streamlined rewrite + restrained fallback dedup + depth-first + negative-review bound, one-off)": {
		"ru": "[prompts] к промпту planner добавлена новая дефолтная версия (упрощённая переработка + аккуратная дедупликация деградаций + приоритет глубины + верхняя граница негативной проверки, разовая миграция)",
		"zh": "[prompts] planner 提示词已追加新默认版本(精简重构+克制降级去重+深度优先+否定复核上界,一次性)",
	},
	"[prompts] failed to re-seed the worker prompt with the new default: %v": {
		"ru": "[prompts] не удалось перезаписать промпт worker новым дефолтом: %v",
		"zh": "[prompts] worker 提示词重刷为新默认失败: %v",
	},
	"[prompts] appended new default version to the worker prompt (context lookup limited to list_assets/list_findings; list_facts/node_detail/asset_neighbors removed, one-off)": {
		"ru": "[prompts] к промпту worker добавлена новая дефолтная версия (поиск контекста сведён к list_assets/list_findings; list_facts/node_detail/asset_neighbors убраны, разовая миграция)",
		"zh": "[prompts] worker 提示词已追加新默认版本(查上下文段收敛为 list_assets/list_findings,去掉 list_facts/node_detail/asset_neighbors,一次性)",
	},
	"[reporter] failed to read triggers: %v": {
		"ru": "[reporter] не удалось прочитать триггеры: %v",
		"zh": "[reporter] 读取触发器失败: %v",
	},
	"[reporter] failed to upgrade trigger message: %v": {
		"ru": "[reporter] не удалось обновить сообщение триггера: %v",
		"zh": "[reporter] 升级触发消息失败: %v",
	},
	"[reporter] trigger message upgraded to read and return evidence_version": {
		"ru": "[reporter] сообщение триггера обновлено: теперь читает и возвращает evidence_version",
		"zh": "[reporter] 触发消息已升级为读取并回传 evidence_version",
	},
	"[reporter] failed to create agent: %v": {
		"ru": "[reporter] не удалось создать агента: %v",
		"zh": "[reporter] 创建 agent 失败: %v",
	},
	"[reporter] failed to seed prompt: %v": {
		"ru": "[reporter] не удалось записать исходный промпт: %v",
		"zh": "[reporter] seed prompt 失败: %v",
	},
	"[reporter] failed to set trigger run policy: %v": {
		"ru": "[reporter] не удалось задать политику запуска по триггеру: %v",
		"zh": "[reporter] 设置触发运行策略失败: %v",
	},
	"[reporter] failed to bind tools: %v": {
		"ru": "[reporter] не удалось привязать инструменты: %v",
		"zh": "[reporter] 绑定工具失败: %v",
	},
	"[reporter] failed to create trigger: %v": {
		"ru": "[reporter] не удалось создать триггер: %v",
		"zh": "[reporter] 创建触发器失败: %v",
	},
	"[auto] failed to bind report_finding by default: %v": {
		"ru": "[auto] не удалось привязать report_finding по умолчанию: %v",
		"zh": "[auto] report_finding 默认绑定失败: %v",
	},
	"[planner] failed to bind report_finding by default: %v": {
		"ru": "[planner] не удалось привязать report_finding по умолчанию: %v",
		"zh": "[planner] report_finding 默认绑定失败: %v",
	},
	"[planner] failed to bind list_assets by default: %v": {
		"ru": "[planner] не удалось привязать list_assets по умолчанию: %v",
		"zh": "[planner] list_assets 默认绑定失败: %v",
	},
	"[planner] failed to bind add_company_scope by default: %v": {
		"ru": "[planner] не удалось привязать add_company_scope по умолчанию: %v",
		"zh": "[planner] add_company_scope 默认绑定失败: %v",
	},
	"[worker] failed to unbind add_company_scope: %v": {
		"ru": "[worker] не удалось отвязать add_company_scope: %v",
		"zh": "[worker] add_company_scope 解绑失败: %v",
	},
	"[worker] failed to unbind %s from worker: %v": {
		"ru": "[worker] не удалось отвязать %s от worker: %v",
		"zh": "[worker] %s 从 worker 解绑失败: %v",
	},
	"[worker] failed to backfill review/detail tool bindings: %v": {
		"ru": "[worker] не удалось довязать инструменты просмотра/деталей: %v",
		"zh": "[worker] 回看/详情工具补绑失败: %v",
	},
	"[auto] default tool binding failed: %v": {
		"ru": "[auto] не удалась привязка инструмента по умолчанию: %v",
		"zh": "[auto] 默认绑定失败: %v",
	},
	"[notify] failed to dispatch event: %v": {
		"ru": "[notify] не удалось отправить событие: %v",
		"zh": "[notify] 分派事件失败: %v",
	},
	"[notify] failed to read channels: %v": {
		"ru": "[notify] не удалось прочитать каналы: %v",
		"zh": "[notify] 读取渠道失败: %v",
	},
	"[notify] failed to claim real-time delivery channel=%d: %v": {
		"ru": "[notify] не удалось занять мгновенную доставку channel=%d: %v",
		"zh": "[notify] 领取实时投递失败 channel=%d: %v",
	},
	"[notify] failed to evaluate digest batch channel=%d: %v": {
		"ru": "[notify] не удалось определить пакет дайджеста channel=%d: %v",
		"zh": "[notify] 判断汇总批次失败 channel=%d: %v",
	},
	"[notify] failed to claim digest batch channel=%d: %v": {
		"ru": "[notify] не удалось занять пакет дайджеста channel=%d: %v",
		"zh": "[notify] 领取汇总批次失败 channel=%d: %v",
	},
	"[notify] failed to mark bad-snapshot deliveries as failed channel=%s ids=%v: %v": {
		"ru": "[notify] не удалось пометить доставки с битыми снимками channel=%s ids=%v: %v",
		"zh": "[notify] 标记坏快照投递失败 channel=%s ids=%v: %v",
	},
	"[notify] skipping %d deliveries whose snapshot cannot be parsed channel=%d": {
		"ru": "[notify] пропущено %d доставок с нечитаемым снимком channel=%d",
		"zh": "[notify] 跳过 %d 条快照无法解析的投递 channel=%d",
	},
	"[notify] channel reported %d delivered of %d deliveries channel=%s, treating all as delivered": {
		"ru": "[notify] канал отчитался о %d доставленных при %d доставках channel=%s, считаем все доставленными",
		"zh": "[notify] 渠道回报送达条数 %d 超过投递数 %d channel=%s，按全部送达处理",
	},
	"[notify] failed to mark deliveries as delivered channel=%s ids=%v: %v": {
		"ru": "[notify] не удалось отметить доставки как доставленные channel=%s ids=%v: %v",
		"zh": "[notify] 标记已送达失败 channel=%s ids=%v: %v",
	},
	"[notify] failed to requeue segmented continuation channel=%s ids=%v: %v": {
		"ru": "[notify] не удалось заново поставить в очередь продолжение сегментов channel=%s ids=%v: %v",
		"zh": "[notify] 分段续发排队失败 channel=%s ids=%v: %v",
	},
	"[notify] failed to mark delivery failure state channel=%s ids=%v: %v": {
		"ru": "[notify] ошибка при отметке статуса неудачи channel=%s ids=%v: %v",
		"zh": "[notify] 标记失败状态出错 channel=%s ids=%v: %v",
	},
	"[notify] failed to reorder deliveries channel=%s ids=%v: %v": {
		"ru": "[notify] не удалось переупорядочить доставки channel=%s ids=%v: %v",
		"zh": "[notify] 重排投递失败 channel=%s ids=%v: %v",
	},
	"[notify] delivery failed channel=%d kind=%s permanently_failed=%d retries_exhausted=%d pending_retry=%d: %s": {
		"ru": "[notify] доставка не удалась channel=%d kind=%s навсегда=%d исчерпано=%d ожидает_повтора=%d: %s",
		"zh": "[notify] 投递失败 channel=%d kind=%s 永久失败=%d 重试耗尽=%d 待重试=%d: %s",
	},
	"[notify] skipped unparseable snapshot in digest batch delivery=%d: %v": {
		"ru": "[notify] пропущен нечитаемый снимок в пакете дайджеста delivery=%d: %v",
		"zh": "[notify] 汇总批次中跳过无法解析的快照 delivery=%d: %v",
	},
	"[notify] failed to parse asset name finding=%d: %v": {
		"ru": "[notify] не удалось разобрать имя актива finding=%d: %v",
		"zh": "[notify] 解析资产名失败 finding=%d: %v",
	},
	"[task] #%s intent #%d reopened (re-run)": {
		"ru": "[task] #%s intent #%d переоткрыт (повторный запуск)",
		"zh": "[task] #%s 意图 #%d 已重开(重跑)",
	},
	"[task] #%s reopened %d blocked intents in bulk": {
		"ru": "[task] #%s массово переоткрыто %d заблокированных intent",
		"zh": "[task] #%s 批量重开 %d 条 blocked 意图",
	},
	"[task] new task #%s «%s» goal: %s": {
		"ru": "[task] создана задача #%s «%s» цель: %s",
		"zh": "[task] 新建任务 #%s «%s» 目标: %s",
	},
	"[seed] task %s: could not parse a target host/IP from %q, no site created (configure scope manually)": {
		"ru": "[seed] задача %s: не удалось извлечь host/IP из %q, сайт не создан (настройте scope вручную)",
		"zh": "[seed] task %s: 未能从 %q 解析出目标 host/IP，不创建站点（请手动配置 scope）",
	},
	"[seed] task %s: target %q is an LLM gateway, refusing to use it as a pentest target": {
		"ru": "[seed] задача %s: цель %q является LLM-шлюзом, отказано в пентесте по ней",
		"zh": "[seed] task %s: 目标 %q 是 LLM 网关，拒绝作为渗透目标",
	},
	"[seed] task %s: target site %s": {
		"ru": "[seed] задача %s: целевой сайт %s",
		"zh": "[seed] task %s: 目标站点 %s",
	},
	"[seed] task %s: failed to issue seed intent: %v": {
		"ru": "[seed] задача %s: не удалось создать стартовый intent: %v",
		"zh": "[seed] task %s: 下发种子意图失败: %v",
	},
	"[seed] task %s: seed intent #%d issued": {
		"ru": "[seed] задача %s: стартовый intent #%d создан",
		"zh": "[seed] task %s: 已下发种子意图 #%d",
	},
	"[notify] finding status change event not recorded finding=%d %s->%s (status already updated)": {
		"ru": "[notify] событие смены статуса не записано finding=%d %s->%s (статус уже обновлён)",
		"zh": "[notify] 状态变更事件未登记 finding=%d %s→%s（状态已更新）",
	},
	"[traffic] blob download %s interrupted: %v": {
		"ru": "[traffic] загрузка blob %s прервана: %v",
		"zh": "[traffic] 下载 blob %s 中断：%v",
	},
	"[task-delete] task %s failed to read persisted state, restoring the pause barrier from memory: %v": {
		"ru": "[task-delete] задача %s не удалось прочитать сохранённое состояние, барьер восстановлен из памяти: %v",
		"zh": "[task-delete] task %s 读取持久状态失败，使用内存状态恢复屏障: %v",
	},
	"[agents] failed to seed starter prompt for %s: %v": {
		"ru": "[agents] не удалось записать стартовый промпт для %s: %v",
		"zh": "[agents] seed starter prompt for %s 失败: %v",
	},
	"[agents] failed to clean up %s tool bindings: %v": {
		"ru": "[agents] не удалось очистить привязки инструментов %s: %v",
		"zh": "[agents] 清理 %s 工具绑定失败: %v",
	},
	"[agents] failed to clean up %s triggers: %v": {
		"ru": "[agents] не удалось очистить триггеры %s: %v",
		"zh": "[agents] 清理 %s 触发器失败: %v",
	},
	"[tools] failed to read call stats: %v": {
		"ru": "[tools] не удалось прочитать статистику вызовов: %v",
		"zh": "[tools] 读取调用统计失败: %v",
	},
	"[mcp] %s tool discovery after add failed: %v": {
		"ru": "[mcp] %s обнаружение инструментов после добавления не удалось: %v",
		"zh": "[mcp] %s 添加后工具发现失败: %v",
	},
	"[skills] failed to read call stats: %v": {
		"ru": "[skills] не удалось прочитать статистику вызовов: %v",
		"zh": "[skills] 读取调用统计失败: %v",
	},
	"[mcp] %s connection failed: %v": {
		"ru": "[mcp] %s не удалось подключиться: %v",
		"zh": "[mcp] %s 连接失败: %v",
	},
	"[mcp] %s tools/list failed: %v": {
		"ru": "[mcp] %s tools/list не удался: %v",
		"zh": "[mcp] %s tools/list 失败: %v",
	},
	"[prompts] seed %s skipped: agent does not exist (%v)": {
		"ru": "[prompts] пропуск seed %s: агент не существует (%v)",
		"zh": "[prompts] seed %s 跳过: agent 不存在 (%v)",
	},
	"[prompts] seed %s failed: %v": {
		"ru": "[prompts] ошибка seed %s: %v",
		"zh": "[prompts] seed %s 失败: %v",
	},
	"[tools] seed %s failed: %v": {
		"ru": "[tools] ошибка seed %s: %v",
		"zh": "[tools] seed %s 失败: %v",
	},
	"[tools] failed to read the tool table, allowing by code defaults: %v": {
		"ru": "[tools] не удалось прочитать таблицу инструментов, разрешение по умолчанию из кода: %v",
		"zh": "[tools] 读取工具表失败，按代码默认放行: %v",
	},
	"[concurrency] task %s failed to start: %v": {
		"ru": "[concurrency] задача %s не удалось запустить: %v",
		"zh": "[concurrency] task %s 启动失败: %v",
	},
	"[concurrency] task %s failed to enqueue due to unavailable LLM: %v": {
		"ru": "[concurrency] задача %s не удалось поставить в очередь из-за недоступного LLM: %v",
		"zh": "[concurrency] task %s 因 LLM 不可用入队失败: %v",
	},
	"[revive] task %s failed to resume: %v": {
		"ru": "[revive] задача %s не удалось возобновить: %v",
		"zh": "[revive] task %s 恢复失败: %v",
	},
	"[goals] task %s: LLM goal breakdown produced nothing, falling back to the raw goal as a single goal": {
		"ru": "[goals] задача %s: LLM не смог разбить цель, используется исходная цель как единственная",
		"zh": "[goals] task %s: LLM 目标拆解无产出，回退为「原始目标作为单目标」",
	},
	"[update] update failed: %v": {
		"ru": "[update] обновление не удалось: %v",
		"zh": "[update] 更新失败：%v",
	},
	"[update] %s -> %s staged, exiting shortly to complete the swap": {
		"ru": "[update] %s -> %s подготовлено, скоро выход для завершения переключения",
		"zh": "[update] %s → %s 已暂存，即将退出以完成换装",
	},
	"[update] manually rolled back to the previous version, exiting shortly to complete the switch": {
		"ru": "[update] выполнен откат на предыдущую версию вручную, скоро выход для завершения переключения",
		"zh": "[update] 已手动回滚到上一版本，即将退出以完成切换",
	},
	"[task-archive] skipping symlink (not supported by the archive, other files unaffected): %s": {
		"ru": "[task-archive] пропуск символической ссылки (не поддерживается архивом, другие файлы не затронуты): %s",
		"zh": "[task-archive] 跳过符号链接（归档不支持，不影响其它文件）：%s",
	},
	"[traffic] incremental index reclamation is not enabled (auto_vacuum=%d): deleting traffic will not shrink index.sqlite, run a storage compaction once to convert": {
		"ru": "[traffic] инкрементальная очистка индекса не включена (auto_vacuum=%d): удаление трафика не уменьшит index.sqlite, нужна однократная компакция хранилища",
		"zh": "[traffic] 索引库未启用增量回收（auto_vacuum=%d）：删除流量不会缩小 index.sqlite，需要执行一次存储压缩来转换",
	},
	"[traffic] full-text index unavailable, body search disabled (metadata search unaffected): %v": {
		"ru": "[traffic] полнотекстовый индекс недоступен, поиск по телу отключён (поиск по метаданным работает): %v",
		"zh": "[traffic] 全文索引不可用，正文搜索将被禁用（元数据搜索不受影响）：%v",
	},
	"[traffic] MITM error with %s, falling back to pass-through (this host will connect directly and is no longer recorded, but requests still work): %v": {
		"ru": "[traffic] ошибка MITM с %s, включён сквозной режим (этот host пойдёт напрямую без записи, запросы продолжат работать): %v",
		"zh": "[traffic] 与 %s 的 MITM 出错，改为透传（该 host 后续直连目标、不再记录，但请求照常）：%v",
	},
	"[traffic] failed to record %s (begin transaction): %v": {
		"ru": "[traffic] не удалось записать %s (открытие транзакции): %v",
		"zh": "[traffic] 记录 %s 失败（开启事务）：%v",
	},
	"[traffic] failed to record %s (write index): %v": {
		"ru": "[traffic] не удалось записать %s (запись индекса): %v",
		"zh": "[traffic] 记录 %s 失败（写索引）：%v",
	},
	"[traffic] failed to record %s (fetch rowid): %v": {
		"ru": "[traffic] не удалось записать %s (получение rowid): %v",
		"zh": "[traffic] 记录 %s 失败（取 rowid）：%v",
	},
	"[traffic] failed to record %s (write body): %v": {
		"ru": "[traffic] не удалось записать %s (запись тела): %v",
		"zh": "[traffic] 记录 %s 失败（写正文）：%v",
	},
	"[traffic] failed to record %s (register blob references): %v": {
		"ru": "[traffic] не удалось записать %s (регистрация ссылок на blob): %v",
		"zh": "[traffic] 记录 %s 失败（登记 blob 引用）：%v",
	},
	"[traffic] failed to record %s (write full-text index): %v": {
		"ru": "[traffic] не удалось записать %s (запись полнотекстового индекса): %v",
		"zh": "[traffic] 记录 %s 失败（写全文索引）：%v",
	},
	"[traffic] failed to record %s (commit): %v": {
		"ru": "[traffic] не удалось записать %s (коммит): %v",
		"zh": "[traffic] 记录 %s 失败（提交）：%v",
	},
	"[traffic] failed to create blob directory: %v": {
		"ru": "[traffic] не удалось создать каталог blob: %v",
		"zh": "[traffic] 创建 blob 目录失败：%v",
	},
	"[traffic] failed to write blob %s: %v": {
		"ru": "[traffic] не удалось записать blob %s: %v",
		"zh": "[traffic] 写 blob %s 失败：%v",
	},
	"[traffic] failed to compact index: %v": {
		"ru": "[traffic] не удалось сжать индекс: %v",
		"zh": "[traffic] 压实索引失败：%v",
	},
	"[traffic] failed to clean up old traffic directory %s: %v": {
		"ru": "[traffic] не удалось очистить старый каталог трафика %s: %v",
		"zh": "[traffic] 清理历史流量目录 %s 失败：%v",
	},
	"[traffic] failed to reclaim index space (get connection): %v": {
		"ru": "[traffic] не удалось освободить место индекса (получение соединения): %v",
		"zh": "[traffic] 回收索引空间失败（获取连接）：%v",
	},
	"[traffic] failed to reclaim index space: %v": {
		"ru": "[traffic] не удалось освободить место индекса: %v",
		"zh": "[traffic] 回收索引空间失败：%v",
	},
	"[traffic] index reclamation incomplete (hit the %d step limit), will continue on the next deletion": {
		"ru": "[traffic] очистка места индекса не завершена (достигнут предел %d шагов), продолжится при следующем удалении",
		"zh": "[traffic] 索引空间回收未做完（已用满 %d 步上限），下次删除时继续",
	},
	"[traffic] index reclamation incomplete (hit the %s budget), will continue on the next deletion": {
		"ru": "[traffic] очистка места индекса не завершена (исчерпан бюджет %s), продолжится при следующем удалении",
		"zh": "[traffic] 索引空间回收未做完（已用满 %s 预算），下次删除时继续",
	},
	"[traffic] failed to truncate WAL: %v": {
		"ru": "[traffic] не удалось усечь WAL: %v",
		"zh": "[traffic] 截断 WAL 失败：%v",
	},
	"[update] bootstrap skipped: %v": {
		"ru": "[update] самонастройка пропущена: %v",
		"zh": "[update] 跳过自举：%v",
	},
	"[update] staged new version failed validation, discarded, continuing the current version: %v": {
		"ru": "[update] подготовленная новая версия не прошла проверку, отброшена, работа текущей версии продолжается: %v",
		"zh": "[update] 暂存的新版本未通过校验，已丢弃，继续运行当前版本：%v",
	},
	"[update] swap failed, continuing the current version: %v": {
		"ru": "[update] переключение не удалось, продолжается работа текущей версии: %v",
		"zh": "[update] 换装失败，继续运行当前版本：%v",
	},
	"[update] failed to write the upgrade marker (automatic rollback is now unavailable): %v": {
		"ru": "[update] не удалось записать метку обновления (автоматический откат больше недоступен): %v",
		"zh": "[update] 写升级标记失败（失去自动回滚能力）：%v",
	},
	"[update] swapped to %s, exiting to restart (exit %d)": {
		"ru": "[update] выполнено переключение на %s, выход для перезапуска (exit %d)",
		"zh": "[update] 已换装到 %s，退出以重启（exit %d）",
	},
	"[update] new version failed to start %d times in a row and rollback failed: %v": {
		"ru": "[update] новая версия %d раз подряд не смогла запуститься, откат также не удался: %v",
		"zh": "[update] 新版本连续 %d 次启动失败，且回滚失败：%v",
	},
	"[update] new version failed to start %d times in a row, rolled back to %s, exiting to restart (exit %d)": {
		"ru": "[update] новая версия %d раз подряд не смогла запуститься, выполнен откат на %s, выход для перезапуска (exit %d)",
		"zh": "[update] 新版本连续 %d 次启动失败，已回滚到 %s，退出以重启（exit %d）",
	},
	"[update] failed to update the upgrade marker: %v": {
		"ru": "[update] не удалось обновить метку обновления: %v",
		"zh": "[update] 更新升级标记失败：%v",
	},
	"[update] new version starting (attempt %d/%d), the upgrade will be confirmed after stable operation": {
		"ru": "[update] запуск новой версии (попытка %d/%d), обновление будет подтверждено после стабильной работы",
		"zh": "[update] 新版本启动中（第 %d/%d 次尝试），稳定运行后将确认升级",
	},
	"[update] failed to clear the upgrade marker: %v": {
		"ru": "[update] не удалось сбросить метку обновления: %v",
		"zh": "[update] 清除升级标记失败：%v",
	},
	"[update] new version is stable, upgrade complete (previous version kept as %s)": {
		"ru": "[update] новая версия работает стабильно, обновление завершено (предыдущая версия сохранена как %s)",
		"zh": "[update] 新版本运行稳定，升级完成（上一版本保留为 %s）",
	},
	"[update] failed to tidy backups after rollback (operation unaffected): %v": {
		"ru": "[update] не удалось привести в порядок бэкапы после отката (на работу не влияет): %v",
		"zh": "[update] 回滚后整理备份失败（不影响运行）：%v",
	},
	"[enrich] dnsx initialization failed, DNS resolution disabled: %v": {
		"ru": "[enrich] инициализация dnsx не удалась, DNS-разрешение отключено: %v",
		"zh": "[enrich] dnsx 初始化失败，DNS 解析停用：%v",
	},
	"[enrich] queue full, dropping job kind=%d id=%d": {
		"ru": "[enrich] очередь заполнена, задача отброшена kind=%d id=%d",
		"zh": "[enrich] 队列已满，丢弃任务 kind=%d id=%d",
	},
	"[llm-test] %s / %s @ %s — no HTTP request was sent (config resolution or connection setup failed)": {
		"ru": "[llm-test] %s / %s @ %s — HTTP-запрос не отправлялся (ошибка разбора конфигурации или установки соединения)",
		"zh": "[llm-test] %s / %s @ %s — 未发出任何 HTTP 请求(配置解析或建连即失败)",
	},
	"[llm-test] %s / %s @ %s — attempt %d/%d HTTP %d\nbody: %s": {
		"ru": "[llm-test] %s / %s @ %s — попытка %d/%d HTTP %d\nтело ответа: %s",
		"zh": "[llm-test] %s / %s @ %s — 尝试 %d/%d HTTP %d\n响应体: %s",
	},
	"[notify] failed to serialize push event finding=%d: %v": {
		"ru": "[notify] не удалось сериализовать push-событие finding=%d: %v",
		"zh": "[notify] 序列化推送事件失败 finding=%d: %v",
	},
	"[notify] failed to create savepoint finding=%d: %v": {
		"ru": "[notify] не удалось создать точку сохранения finding=%d: %v",
		"zh": "[notify] 建立保存点失败 finding=%d: %v",
	},
	"[notify] failed to write push event finding=%d (finding record unaffected): %v": {
		"ru": "[notify] не удалось записать push-событие finding=%d (на запись находки не влияет): %v",
		"zh": "[notify] 写入推送事件失败 finding=%d（漏洞记录不受影响）: %v",
	},
	"[notify] failed to roll back to savepoint finding=%d: %v": {
		"ru": "[notify] не удалось откатить к точке сохранения finding=%d: %v",
		"zh": "[notify] 回滚到保存点失败 finding=%d: %v",
	},
	"Scheduled trigger · %s": {
		"ru": "Периодический триггер · %s",
		"zh": "定时触发 · %s",
	},
	"finding trigger · task#%d": {
		"ru": "триггер finding · task#%d",
		"zh": "finding 触发 · task#%d",
	},
	"goal trigger · task#%d": {
		"ru": "триггер цели · task#%d",
		"zh": "目标触发 · task#%d",
	},
	"timeout trigger · task#%d": {
		"ru": "триггер таймаута · task#%d",
		"zh": "超时触发 · task#%d",
	},
	"Tool trigger · %s · task#%d": {
		"ru": "Триггер инструмента · %s · task#%d",
		"zh": "工具触发 · %s · task#%d",
	},
	"LLM failover chain: no usable profile available": {
		"ru": "Цепь отказоустойчивости LLM: нет доступных профилей",
		"zh": "LLM 轮询链：没有可用的配置",
		"ko": "LLM 폴백 체인: 사용 가능한 설정이 없습니다",
	},
	"facts=%d assets=%d findings=%d": {
		"ru": "факты=%d активы=%d находки=%d",
		"zh": "事实%d 资产%d 漏洞%d",
	},
}

// trCfg translates a startup/operator message to the configured default language
// (Language()). Format templates are translated the same way: pass the Korean
// template through trCfg before fmt.Sprintf/fmt.Errorf.
func trCfg(ko string) string {
	if m, ok := cfgCatalog[ko]; ok {
		if v := Pick(m, Language()); v != "" {
			return v
		}
	}
	return ko
}

// T translates an operator-facing console log template to the configured default
// language (Language()). Exposed for cmd and server log.Printf call sites. Format
// verbs are preserved in order, so wrap the template only: log.Printf(config.T(ko), args...).
func T(ko string) string {
	if m, ok := logCatalog[ko]; ok {
		if v := Pick(m, Language()); v != "" {
			return v
		}
	}
	return ko
}

// TA translates an operator-facing console log template to the runtime active
// language (settings table layered over the bootstrap default) — the same
// resolution backend display strings use, unlike T which resolves only the
// bootstrap default. New operator-facing log sites wrap English templates:
// log.Printf(config.TA(template), args...). A template missing from the catalog
// (or a language without an entry) resolves to the template itself, so English
// stays the baseline.
func TA(template string) string {
	if m, ok := logCatalog[template]; ok {
		if v := Pick(m, ActiveLanguage()); v != "" {
			return v
		}
	}
	return template
}
