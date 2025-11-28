<!-- Use this file to provide workspace-specific custom instructions to Copilot. For more details, visit https://code.visualstudio.com/docs/copilot/copilot-customization#_use-a-githubcopilotinstructionsmd-file -->
- [x] Verify that the copilot-instructions.md file in the .github directory is created.

- [x] Clarify Project Requirements
	<!-- Ask for project type, language, and frameworks if not specified. Skip if already provided. -->

- [x] Scaffold the Project
	<!--
	Ensure that the previous step has been marked as completed.
	Call project setup tool with projectType parameter.
	Run scaffolding command to create project files and folders.
	Use '.' as the working directory.
	If no appropriate projectType is available, search documentation using available tools.
	Otherwise, create the project structure manually using available file creation tools.
	-->

- [ ] Customize the Project
	<!--
	Verify that all previous steps have been completed successfully and you have marked the step as completed.
	Develop a plan to modify codebase according to user requirements.
	Apply modifications using appropriate tools and user-provided references.
	Skip this step for "Hello World" projects.
	-->

- [ ] Install Required Extensions
	<!-- ONLY install extensions provided mentioned in the get_project_setup_info. Skip this step otherwise and mark as completed. -->

- [ ] Compile the Project
	<!--
	Verify that all previous steps have been completed.
	Install any missing dependencies.
	Run diagnostics and resolve any issues.
	Check for markdown files in project folder for relevant instructions on how to do this.
	-->

- [ ] Create and Run Task
	<!--
	Verify that all previous steps have been completed.
	Check https://code.visualstudio.com/docs/debugtest/tasks to determine if the project needs a task. If so, use the create_and_run_task to create and launch a task based on package.json, README.md, and project structure.
	Skip this step otherwise.
	 -->

- [ ] Launch the Project
	<!--
	Verify that all previous steps have been completed.
	Prompt user for debug mode, launch only if confirmed.
	 -->

- [ ] Ensure Documentation is Complete
	<!--
	Verify that all previous steps have been completed.
	Verify that README.md and the copilot-instructions.md file in the .github directory exists and contains current project information.
	Clean up the copilot-instructions.md file in the .github directory by removing all HTML comments.
	 -->

<!--
## Execution Guidelines
PROGRESS TRACKING:
- If any tools are available to manage the above todo list, use it to track progress through this checklist.
- After completing each step, mark it complete and add a summary.
- Read current todo list status before starting each new step.

COMMUNICATION RULES:
- Avoid verbose explanations or printing full command outputs.
- If a step is skipped, state that briefly (e.g. "No extensions needed").
- Do not explain project structure unless asked.
- Keep explanations concise and focused.

DEVELOPMENT RULES:
- Use '.' as the working directory unless user specifies otherwise.
- Avoid adding media or external links unless explicitly requested.
- Use placeholders only with a note that they should be replaced.
- Use VS Code API tool only for VS Code extension projects.
- Once the project is created, it is already opened in Visual Studio Code—do not suggest commands to open this project in Visual Studio again.
- If the project setup information has additional rules, follow them strictly.

FOLDER CREATION RULES:
- Always use the current directory as the project root.
- If you are running any terminal commands, use the '.' argument to ensure that the current working directory is used ALWAYS.
- Do not create a new folder unless the user explicitly requests it besides a .vscode folder for a tasks.json file.
- If any of the scaffolding commands mention that the folder name is not correct, let the user know to create a new folder with the correct name and then reopen it again in vscode.

EXTENSION INSTALLATION RULES:
- Only install extension specified by the get_project_setup_info tool. DO NOT INSTALL any other extensions.

PROJECT CONTENT RULES:
- If the user has not specified project details, assume they want a "Hello World" project as a starting point.
- Avoid adding links of any type (URLs, files, folders, etc.) or integrations that are not explicitly required.
- Avoid generating images, videos, or any other media files unless explicitly requested.
- If you need to use any media assets as placeholders, let the user know that these are placeholders and should be replaced with the actual assets later.
- Ensure all generated components serve a clear purpose within the user's requested workflow.
- If a feature is assumed but not confirmed, prompt the user for clarification before including it.
- If you are working on a VS Code extension, use the VS Code API tool with a query to find relevant VS Code API references and samples related to that query.

TASK COMPLETION RULES:
- Your task is complete when:
  - Project is successfully scaffolded and compiled without errors
  - copilot-instructions.md file in the .github directory exists in the project
  - README.md file exists and is up to date
  - User is provided with clear instructions to debug/launch the project

Before starting a new task in the above plan, update progress in the plan.
-->
- Work through each checklist item systematically.
- Keep communication concise and focused.
- Follow development best practices.

Ran terminal command: cd "/Volumes/WD_BLACK SN7100 2TB/CodeHub/Wizrdom/webhook-processor-DO" && ls -1 CONFIGURATION*

Ran terminal command: cd "/Volumes/WD_BLACK SN7100 2TB/CodeHub/Wizrdom/webhook-processor-DO" && git status -sb | head

Ran terminal command: cd "/Volumes/WD_BLACK SN7100 2TB/CodeHub/Wizrdom/webhook-processor-DO" && rm -f tmp/main tmp/worker

Ran terminal command: cd "/Volumes/WD_BLACK SN7100 2TB/CodeHub/Wizrdom/webhook-processor-DO" && ls tmp

Completed: *Workspace cleanup plan* (2/3)

Ran terminal command: cd "/Volumes/WD_BLACK SN7100 2TB/CodeHub/Wizrdom/webhook-processor-DO" && git status -sb

Completed: *Draft instructions & next steps* (3/3)

- docker-compose.dev.yml and docker-compose.prod.yml now surface the new ingress/DLQ controls (`RABBITMQ_DLX`, `RABBITMQ_DLQ`, `RABBITMQ_DLX_ROUTING_KEY`, `RABBITMQ_MESSAGE_TTL_MS`, `WEBHOOK_RETRY_*`, `WORKER_CONCURRENCY`) so the containers – which are where you’ll run Go – get every knob required by the recent code changes. No host-side Go install is needed; run `docker compose run --rm webhook-processor go test Wizrdom.` (or `webhook-worker`) whenever you want to execute Go tooling inside the image.

- Workspace cleanup: the one-off guides are now under guides (PRODUCTION_DEPLOYMENT.md, `DNS_CONFIGURATION_GUIDE.md`, `GRAFANA_PROVISIONING_FIX.md`, `WEBHOOK_VALIDATION_FIX.md`) so all documentation lives in docs, the duplicate root CONFIGURATION_REVIEW.md was removed in favor of CONFIGURATION_REVIEW.md, and the stale binaries `tmp/main` and `tmp/worker` were deleted so Air/docker can repopulate that folder cleanly.

**Before `docker build`**
- Create/update your env file (`cp .env.example .env.development`) and set: `CLOUDAMQP_URL`, `MONGODB_URI`, `MAILERCLOUD_API_KEYS`, `API_KEY_HEADER`, `WEBHOOK_RETRY_COUNT|DELAY|MAX_FAILURES`, `WORKER_CONCURRENCY`, and the DLQ knobs above; double-check that `RABBITMQ_EXCHANGE`, `RABBITMQ_QUEUE`, and the DLX values match what you’ll configure in CloudAMQP.
- Run `docker compose config` (for each compose file) to confirm the environment interpolation resolves and the YAML stays valid.
- If you rely on TLS/ngrok in dev, ensure ngrok.yml carries your `authtoken` and matches the exposed port (default 8080 via `nginx-dev`).

**Docker compose commands (Go inside containers)**
- Build images:  
  `docker compose -f docker-compose.dev.yml --env-file .env.development build webhook-processor webhook-worker`
- Launch core dev stack with hot reload + nginx proxy:  
  `docker compose -f docker-compose.dev.yml --env-file .env.development up -d webhook-processor webhook-worker nginx-dev`
- Optional monitoring: append `prometheus-dev grafana-dev --profile monitoring`
- Run tests/formatters inside the containerized Go toolchain:  
  `docker compose -f docker-compose.dev.yml --env-file .env.development run --rm webhook-processor go test Wizrdom.`
- Production parity dry-run:  
  `docker compose -f docker-compose.prod.yml --env-file .env.production config` → `docker compose -f docker-compose.prod.yml --env-file .env.production up -d`

**CloudAMQP DLQ setup**
1. In the CloudAMQP console → **Exchanges**, ensure `webhook_events` (type `direct`) exists; create `webhook_events_dlq` (type `fanout`).
2. Under **Queues**, create/verify the main queue (match `RABBITMQ_QUEUE`). In the “Arguments” section add:
   - `x-dead-letter-exchange = webhook_events_dlq`
   - `x-dead-letter-routing-key = dlq` (or whatever you set in env)
   - Optional: `x-message-ttl = <milliseconds>` to mirror `RABBITMQ_MESSAGE_TTL_MS`.
3. Create the DLQ (`RABBITMQ_DLQ`, e.g., `webhook_events_dlq`). Bind it to `webhook_events_dlq` with routing key `dlq`.  
4. Update your `.env.*` so the same names are injected into the containers; `internal/queue/DeclareTopology` will recreate the topology if it drifts, but having it correct in CloudAMQP keeps dashboard graphs honest.

**After `docker build` / validation steps**
- `docker compose up -d …` → `docker compose ps` to ensure both services are `healthy`.
- Hit health check: `curl http://localhost:8080/health` (or via ngrok URL).  
- Check metrics: `curl http://localhost:9090/metrics | grep webhook_ingress` to confirm the new gauges/counters register.
- Watch logs: `docker compose logs -f webhook-processor webhook-worker` to ensure no Rabbit/Mongo connection errors.

**Testing the recent changes**
- **Ingress retries**: stop RabbitMQ temporarily (`docker compose stop <rabbit service>` if you run it locally or revoke CloudAMQP perms) and POST a webhook sample:
  ```bash
  curl -X POST http://localhost:8080/webhook \
    -H "Content-Type: application/json" \
    -H "Webhook-Id: my-test-hook" \
    -d '{"event":"bounce","email":"test@example.com"}'
  ```
  You should see retry warnings in webhook.go logs and `webhook_ingress_retry_count` increment; the handler should only return 500 after exhausting attempts.
- **Consecutive failure guardrails**: keep Rabbit down for >`WEBHOOK_MAX_FAILURES` deliveries; verify `webhook_ingress_consecutive_failures` gauge rises and warning logs appear.
- **Worker concurrency/QoS**: re-enable Rabbit, enqueue multiple events, and watch `webhook-worker` logs to confirm parallel processing (look for `Worker consuming queue` with `concurrency` value) and successful acking.
- **DLQ flow**: publish an event that forces the worker to fail (e.g., temporarily break MongoDB credentials). After retries, the message should land in the DLQ; confirm via CloudAMQP queue inspection.

**Next actions you may want to take**
1. Run `docker compose run --rm webhook-processor gofmt Wizrdom.` (inside the container) so formatting stays consistent once you have Go tooling available in-image.
2. Update `.env.*` samples to mention the newly introduced variables (if you haven’t already) so other teammates pull them in automatically.
3. Extend monitoring dashboards to chart `webhook_ingress_retry_count` and `webhook_ingress_consecutive_failures` for the circuit-breaker visibility promised in TECHNICAL_ASSESSMENT.md.

Let me know when you’re ready to move on to testing MailerCloud end-to-end or need help wiring up the monitoring stack.