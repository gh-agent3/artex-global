package agent

import "github.com/Autumn-27/norma/harness"

// abortL10n is the user-facing short line and the longer explanation, keyed by
// AbortCause.Code. The Chinese originals stay on the cause values themselves.
var abortL10n = map[string]map[string][2]string{
	"en": {
		"paused_by_user":         {"the user paused the task", "The user paused the task via POST /api/tasks/{id}/control action=pause. This Planner/Worker run was cancelled; in-flight intents return to frontier (open) and run again from the start after resume."},
		"paused_by_orchestrator": {"the orchestrator paused the task", "The orchestrator called pause_task. This Planner/Worker run was cancelled; in-flight intents return to frontier (open) and run again after resume."},
		"task_deleted":           {"the task was deleted", "The task is being deleted (DELETE /api/tasks/{id}). The delete barrier cancelled its Planner, Worker and main agent; this run's result will not be used."},
		"paused_on_reload":       {"the backend restored the paused state", "On startup the backend restored the persisted pause. This run was cancelled; a normal restore has no agent still running."},
		"goal_met":               {"the planner marked the goals done", "The planner decided the goals were met, set the task to done, and cancelled workers still running. Those intents are marked stopped, not failed."},
		"settle_drain_timeout":   {"the timeout drain expired", "After timeout the engine waited for running workers to finish, but the 90s drain was not enough, so it hard-cancelled. Intents are marked exhausted; facts and assets already written are kept."},
		"killed_by_planner":      {"the planner killed this intent", "The planner called kill_work, usually because the direction drifted or had no further value. The intent is marked stopped and is not picked up again."},
		"work_paused_by_user":    {"the user paused this worker intent", "The user paused a running worker. This call was cancelled and the intent is paused; recorded intents, facts, findings and activity are kept and rerun from the start after resume."},
		"work_cancelled_by_user": {"the user deleted this worker intent", "The user deleted a running worker. This call was cancelled. After the worker leaves its write section, the server applies the chosen delete mode."},
		"work_finished":          {"the worker finished and released its context", "The worker finished normally and the engine released its context in detachWork. This is not an interrupt; if it shows up in an interrupt notice, cancel and teardown raced."},
		"paused_race_guard":      {"a new run was refused while the task is paused", "While the task is paused the engine refuses a new execution context, so a claim cannot race the pause and start a worker. A claimed intent returns to frontier."},
		"chat_stopped_by_user":   {"the user stopped this turn", "The user pressed stop and cancelled this main-agent or session turn. Activity already written is kept; the next message can be sent."},
		"chat_paused_with_task":  {"pausing the task also stopped the main-agent turn", "Pausing the task also cancelled the main-agent turn that was in progress. Activity already written is kept; resuming the task does not replay this turn."},
		"chat_turn_finished":     {"this turn finished and its context was released", "The turn finished normally and the server is releasing its context. This is not an interrupt; if it shows up in an interrupt notice, cancel and teardown raced."},
		"shutdown":               {"the backend process is shutting down", "The backend received SIGINT or SIGTERM and is restarting, updating or stopping. Every running agent is cancelled; leftover running intents are reset to open and rerun after restart."},
		"run_hard_timeout":       {"the hard timeout for this run fired", "The run exceeded the soft wall-clock budget plus grace, so a model request or a tool never returned and the normal turn-boundary teardown could not run. Check the last tool call that did not return."},
		"deadline_exceeded":      {"the upstream context hit its deadline", "The upstream context hit its deadline, and the caller did not attach a named cause via WithTimeoutCause."},
		"canceled_no_cause":      {"the canceller attached no named cause", "The upstream context was cancelled without context.WithCancelCause. Register the cause in agent/cancelcause.go and attach it at that cancel site."},
	},
	"ru": {
		"paused_by_user":         {"пользователь поставил задачу на паузу", "Пользователь поставил задачу на паузу через POST /api/tasks/{id}/control, action=pause. Текущий запуск Planner/Worker отменён; незавершённые намерения возвращаются в frontier (open) и после возобновления выполняются с начала."},
		"paused_by_orchestrator": {"оркестратор поставил задачу на паузу", "Оркестратор вызвал pause_task. Текущий запуск Planner/Worker отменён; незавершённые намерения возвращаются в frontier (open) и после возобновления выполняются снова."},
		"task_deleted":           {"задача удаляется", "Задача удаляется (DELETE /api/tasks/{id}). Барьер удаления отменил её Planner, Worker и основного агента; результат этого запуска использован не будет."},
		"paused_on_reload":       {"бэкенд восстановил паузу задачи", "При старте бэкенд восстановил паузу из базы. Этот запуск отменён; при обычном восстановлении работающего агента уже нет."},
		"goal_met":               {"планировщик счёл цели достигнутыми", "Планировщик счёл цели достигнутыми, перевёл задачу в done и отменил ещё работавших Worker. Эти намерения помечаются stopped, а не провалом."},
		"settle_drain_timeout":   {"истекло ожидание завершения по таймауту", "После timeout движок ждал, пока Worker завершатся, но 90 секунд не хватило, и он отменил их жёстко. Намерения помечаются exhausted; уже записанные факты и активы сохраняются."},
		"killed_by_planner":      {"планировщик остановил это намерение", "Планировщик вызвал kill_work и остановил намерение: обычно направление ушло в сторону или продолжать его незачем. Намерение помечается stopped и само снова не берётся."},
		"work_paused_by_user":    {"пользователь поставил этот Worker на паузу", "Пользователь поставил на паузу работающий Worker. Вызов отменён, намерение переходит в paused; намерения, факты, находки и активность сохраняются и после возобновления выполняются с начала."},
		"work_cancelled_by_user": {"пользователь удалил этот Worker", "Пользователь удалил работающий Worker. Вызов отменён; когда Worker выйдет из секции записи, сервер применит выбранный режим удаления."},
		"work_finished":          {"Worker нормально завершился и освободил context", "Worker нормально завершился, движок освобождает его context в detachWork. Это не прерывание; если причина попала в сообщение о прерывании, отмена и завершение разошлись по времени."},
		"paused_race_guard":      {"новый запуск отклонён, пока задача на паузе", "Пока задача на паузе, движок не выдаёт новый context выполнения, чтобы Worker не стартовал из-за гонки claim и паузы. Уже взятое намерение возвращается в frontier."},
		"chat_stopped_by_user":   {"пользователь остановил этот ход", "Пользователь нажал стоп и отменил ход основного агента или сессии. Уже записанная активность сохраняется, следующее сообщение можно отправить."},
		"chat_paused_with_task":  {"пауза задачи прервала ход основного агента", "При паузе задачи отменяется и идущий ход основного агента. Уже записанная активность сохраняется; после возобновления этот ход сам не повторяется."},
		"chat_turn_finished":     {"ход нормально завершился, context освобождён", "Ход нормально завершился, сервер освобождает его context. Это не прерывание; если причина попала в сообщение о прерывании, отмена и завершение разошлись по времени."},
		"shutdown":               {"процесс бэкенда завершается", "Бэкенд получил SIGINT или SIGTERM и перезапускается, обновляется или останавливается. Все работающие агенты отменяются; после старта оставшиеся running-намерения сбрасываются в open и выполняются снова."},
		"run_hard_timeout":       {"сработал жёсткий таймаут этого запуска", "Запуск превысил мягкий предел по времени и дополнительную отсрочку: запрос к модели или инструмент долго не возвращался, и обычное завершение на границе хода не сработало. Смотреть последний вызов инструмента без результата."},
		"deadline_exceeded":      {"у вышестоящего context истёк deadline", "У вышестоящего context истёк deadline, а вызывающий не приложил именованную причину через WithTimeoutCause."},
		"canceled_no_cause":      {"отмена без именованной причины", "Вышестоящий context отменён без context.WithCancelCause. Причину нужно зарегистрировать в agent/cancelcause.go и передать в точке отмены."},
	},
}

var hintL10n = map[string]map[harness.TerminalReason]string{
	"en": {
		harness.ReasonCompleted:         "the model ended the turn normally but left no text summary; facts and assets are whatever this turn's tool calls recorded",
		harness.ReasonMaxTurns:          "hit the step cap (MaxTurns): the SDK finished up and wrote facts and assets back. The intent is marked exhausted so the planner can change direction, not treated as a failure",
		harness.ReasonTimeout:           "hit the wall-clock budget (MaxDuration): the running tool is interrupted and facts and assets found so far are written back. The intent is marked exhausted",
		harness.ReasonModelError:        "the model or API call failed (network, auth, rate limit, provider 5xx). After retries the intent is marked blocked; check the trace before redispatching",
		harness.ReasonBlockingLimit:     "the context length hit the hard cap and the request was blocked before send; narrow the intent or compress tool output",
		harness.ReasonPromptTooLong:     "the prompt is too long and context-compression retries are used up, so execution cannot continue",
		harness.ReasonImageError:        "this model cannot take the multimodal content in this turn; switch to a vision model or stop tools from returning images",
		harness.ReasonStopHookPrevented: "a Stop hook blocked the end of the turn and it could not continue; check whether the task Guard rules are too strict",
		harness.ReasonHookStopped:       "a tool or hook stopped execution, for example an out-of-scope target or a denied command; check the last tool_result",
		harness.ReasonAbortedStreaming:  "the run was cancelled while the model was still writing",
		harness.ReasonAbortedTools:      "the run was cancelled while a tool was still running",
	},
	"ru": {
		harness.ReasonCompleted:         "модель нормально закончила ход, но не оставила текстовый итог; факты и активы — то, что записали вызовы инструментов этого хода",
		harness.ReasonMaxTurns:          "достигнут предел шагов (MaxTurns): SDK завершил ход и записал факты и активы. Намерение помечается exhausted, чтобы планировщик сменил направление, а не как провал",
		harness.ReasonTimeout:           "достигнут предел по времени (MaxDuration): текущий инструмент прерывается, уже найденные факты и активы записываются. Намерение помечается exhausted",
		harness.ReasonModelError:        "вызов модели или API не удался (сеть, авторизация, лимит, 5xx провайдера). После повторов намерение помечается blocked; перед повторной отправкой смотреть трассу",
		harness.ReasonBlockingLimit:     "длина контекста упёрлась в жёсткий предел, запрос не был отправлен; сузить намерение или сжать вывод инструментов",
		harness.ReasonPromptTooLong:     "промпт слишком длинный, повторы сжатия контекста исчерпаны, продолжить нельзя",
		harness.ReasonImageError:        "эта модель не принимает мультимодальное содержимое этого хода; сменить модель на поддерживающую изображения или не возвращать картинки из инструментов",
		harness.ReasonStopHookPrevented: "хук Stop не дал ходу завершиться, и продолжить не удалось; проверить, не слишком ли жёсткие правила Guard",
		harness.ReasonHookStopped:       "инструмент или хук остановил выполнение, например цель вне области или запрещённая команда; смотреть последний tool_result",
		harness.ReasonAbortedStreaming:  "запуск отменён, пока модель ещё писала ответ",
		harness.ReasonAbortedTools:      "запуск отменён, пока ещё выполнялся инструмент",
	},
}
