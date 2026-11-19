# Telegram and Discord Message Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve Pocket Sentinel message readability, reduce manual typing with reusable templates, and document the template rules in a project skill.

**Architecture:** Keep message content in the existing Go bot/alerts layers and the existing Alpine Relay component. Add Telegram copy-text buttons for safe task drafts, editable Relay templates, platform-safe alert formatting, and a model-invoked local skill referenced by AGENTS.md.

**Tech Stack:** Go, Telegram Bot API inline keyboards, Telegram HTML, Discord Markdown, HTML, Alpine.js.

**Spec:** `docs/superpowers/specs/2026-11-19-telegram-discord-message-templates-design.md`

## Global Constraints

- Keep Telegram Bot API and existing command semantics unchanged.
- Copying a `/task` template never starts a task; the user reviews and sends it.
- Escape dynamic text for Telegram HTML and Discord Markdown.
- Do not add dependencies or persist custom templates.
- Preserve Relay PIN, upload, clipboard, and AI Proofread behavior.

## Review Focus

- Copy-text keyboard JSON: Telegram sees exactly one supported button action and does not run a task on tap.
- Dynamic alert text: format remains readable and markup-safe when rendered in Telegram and Discord.
- Empty or long composer text: selecting a template replaces the draft predictably and still allows editing before sending.
- Legacy `/tele-bridge`: both the Dashboard tab and legacy page provide the same template choices.
- User text containing Markdown/HTML characters: no accidental mentions, broken tags, or formatting injection.

## File Structure

- Create `.agents/skills/telegram-discord-message-templates/SKILL.md` for template authoring rules and examples.
- Modify `AGENTS.md` to point agents to the skill for Telegram/Discord message and template work.
- Modify `modules/bot/models.go` with Telegram `copy_text` button representation.
- Modify `modules/bot/keyboards.go` with a help keyboard that retains current direct safe actions and adds copyable commands.
- Modify `modules/bot/router.go` to use concise help sections and the new template keyboard.
- Modify `modules/dashboard/static/js/components/relay.js` with the named editable Relay templates.
- Modify `modules/dashboard/index.html` and `modules/dashboard/tele-bridge.html` to show template picker controls in both Relay views.
- Modify `modules/dashboard/static/css/styles.css` for compact, responsive template controls.
- Modify `modules/dashboard/sw.js` to bump `CACHE_NAME` after frontend edits.
- Modify `modules/alerts/dispatcher.go` to produce a consistent alert/recovery template.
- Modify `main.go` and `modules/bot/router.go` to produce consistent startup, shutdown, and reboot notices.
- Modify `modules/alerts/discord.go` only if needed to preserve the shared template's formatting safely.

## Tasks

### Task 1: Add the message-template skill

- [x] Create a model-invoked SKILL with Telegram HTML, Discord Markdown, placeholder, escaping, length, and review rules; include examples for help, status, alert/recovery, and task drafts.
- [x] Add a precise `AGENTS.md` pointer that triggers the SKILL when creating or changing Telegram/Discord messages, alerts, command menus, or Relay templates.

### Task 2: Improve Telegram help and command shortcuts

- [x] Add `CopyTextButton` and optional `CopyText` to Telegram inline keyboard models using the official `copy_text` JSON field.
- [x] Add a help keyboard with existing direct safe controls and copy-to-clipboard templates for `/task [low]` and `/task [medium]` examples, `/task_status`, `/digest`, and `/exec df -h`.
- [x] Reformat `/start` and `/help` into concise Thai sections with `<code>` commands and a short instruction explaining that task buttons copy drafts.
- [x] Keep `MakeStatusKeyboard()` unchanged so existing direct status behavior remains stable.

### Task 3: Add editable Relay message templates

- [x] Add four templates to `relayComponent`: note, incident report, progress update, and diagnostic request.
- [x] Add a compact selector/button row to both Relay HTML views; selection fills `messageText` and focuses the textarea for editing.
- [x] Keep file mode, paste upload, proofread preview, and send actions unchanged.
- [x] Bump `CACHE_NAME` in `modules/dashboard/sw.js`.

### Task 4: Standardize alert and recovery formatting

- [x] Format Telegram alert and recovery messages with consistent title, condition label, detail, and Bangkok-local timestamp.
- [x] Keep HTML markup compatible with `convertHTMLToDiscordMarkdown` so the same template reads cleanly as Discord Markdown.
- [x] Keep transition logic, alert thresholds, and delivery behavior unchanged.

### Task 5: Review the changes

- [x] Inspect all templates for readability, escaping, and action safety.
- [x] Run `gofmt` for touched Go files, `go build ./...`, and `git diff --check`; do not deploy or restart the server.
