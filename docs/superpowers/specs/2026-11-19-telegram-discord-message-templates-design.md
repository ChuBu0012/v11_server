# Telegram and Discord Message Templates Design

## Intent

Make Pocket Sentinel messages easier to scan and common Telegram tasks easier to start without typing command text from scratch. Establish a reusable project skill for creating and updating Telegram and Discord message templates.

## Goals

- Reformat the Telegram `/start` and `/help` message with clear sections and concise Thai guidance.
- Add one-tap read-only actions and copy-to-clipboard task command templates in Telegram.
- Add selectable, editable message templates to the Telegram Relay composer.
- Give alert and recovery notifications a consistent title, details, and timestamp layout that renders well as Telegram HTML and Discord Markdown.
- Create `.agents/skills/telegram-discord-message-templates/SKILL.md` and make `AGENTS.md` point to it for future template work.

## Non-goals

- Do not change what existing commands execute or run autonomous tasks when a template button is tapped.
- Do not add a Discord interaction UI; Discord remains a webhook notification target.
- Do not add new dependencies or persist user-created templates.

## Constraints

- Use Telegram HTML parse mode and Discord Markdown output.
- Escape dynamic/user-entered text for its destination format.
- Keep Telegram template buttons editable by copying a draft command, not executing it.
- Use the existing Alpine component and preserve text/file sending, PIN, and AI Proofread behavior.
- Keep message templates short, labeled, and easy to copy on mobile.

## Approach

Add a project-local template-writing skill with platform-specific rules, named placeholders, format limits, and review criteria. Replace the single long Telegram help list with short sections and an inline keyboard: existing safe status/torch/ping controls remain actions, while task command examples use Telegram's `copy_text` button field so they can be reviewed before sending. Add a small quick-template selector to both Relay composer entry points; selecting a template places editable Thai text in the current composer. Format alerts once as structured HTML with clear title, detail, and local timestamp, relying on the existing Discord HTML-to-Markdown conversion for Discord delivery.

## Builder decisions

- Telegram `/task` templates copy text to the user's clipboard, since tapping a template must not dispatch an autonomous action.
- Relay templates cover a concise note, incident report, progress update, and diagnostic request; users edit all placeholders before sending.
- The SKILL is model-invoked for future work and is linked from `AGENTS.md` so agents reach it when changing Telegram/Discord message text.
