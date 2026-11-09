# send_email_through_bptickets.py

Send an email, or reply to an existing conversation, through the NBIS ticketing
system (FreeScout, https://tickets.nbis.se). Every email is created as a
conversation in a mailbox, so a record of what was sent stays in the ticketing
system.

The script is generic: it takes any recipient, subject and body. Other scripts
(for example `submit_dataset.sh`) call it to send their messages.

## Setup

Set the credentials in the environment. The API key is never read from
arguments or from a YAML file.

```bash
export TICKET_API_KEY=your_api_key     # FreeScout: Manage -> API & Webhooks
export TICKET_MAILBOX_ID=2             # number after /mailbox/ in the mailbox URL
export TICKET_USER_ID=9                # number in /users/profile/<id> (your agent id)
export TICKET_URL=https://tickets.nbis.se   # optional, this is the default
```

- `TICKET_PASSWORD` is accepted as an alias for `TICKET_API_KEY`.
- The mailbox id is the first number in a URL like `/mailbox/2/13`; the second
  number is the folder and is not needed.
- The user id must be an agent (`/users/profile/<id>`), not a customer
  (`/customers/<id>/edit`).
- `--config` needs PyYAML (`pip install pyyaml`).

## Dry run is the default

Nothing is sent unless you pass `--send`. Without it the script prints the
message and exits, so you can check it first.

## Send a new email

```bash
# 1. Dry run: prints the email, sends nothing
./send_email_through_bptickets.py --to jane@example.org --subject "Test message" --body-file msg.txt

# 2. Send it
./send_email_through_bptickets.py --to jane@example.org --subject "Test message" --body-file msg.txt --send
```

The script prints `sent to jane@example.org, conversation #<id>`. The
conversation is assigned to you and visible in the mailbox.

Other ways to give the body and options:

```bash
# Body from stdin
echo "Short note" | ./send_email_through_bptickets.py --to jane@example.org --subject "Note" --send

# File the conversation as Closed instead of leaving it Active
./send_email_through_bptickets.py --to jane@example.org --subject "FYI" --body-file msg.txt --status closed --send

# HTML body
./send_email_through_bptickets.py --to jane@example.org --subject "Report" --body-file msg.html --html --send
```

Plain-text bodies are HTML-escaped and keep their line breaks.

## Reply to an existing conversation

`--reply-to` takes the conversation id (the number in its URL, for example
`.../conversation/1234`). The reply is added as a message from you and emailed
to the conversation's customer. `--to` and `--subject` are not needed.

```bash
./send_email_through_bptickets.py --reply-to 1234 --body-file reply.txt --send

# Reply and close the conversation
./send_email_through_bptickets.py --reply-to 1234 --body-file reply.txt --status closed --send
```

For a reply, the conversation status stays unchanged unless you give `--status`.

After the reply, the conversation is **assigned to you** (`TICKET_USER_ID`), so it shows
up under Mine. Use `--no-assign` (or `assign: false` in YAML) to leave the assignee as it
is. If the assignment fails, the error says the reply was already sent, with the
conversation id; it is not sent again. New emails are always assigned to the sender.

## Rich text (HTML) body

Write the body as HTML and add `--html` (or `html: true` in YAML). The body is sent as
it is, so use `<p>` or `<br>` for line breaks; plain newlines are ignored.

```bash
cat > msg.html <<'EOF'
<p>Dear Jane,</p>
<p>We found these datasets in the <b>Inbox</b>:</p>
<ul><li><code>DATASET_ED8D4BF</code></li><li><code>DATASET_7CE853F</code></li></ul>
<p>Please <b>reply within 24 hours</b>. See the <a href="https://example.org/docs">guide</a>.</p>
<p>Best regards,<br>BP-ops team</p>
EOF

./send_email_through_bptickets.py --to jane@example.org --subject "Final notice" \
    --body-file msg.html --html --send
```

Keep the HTML simple and use inline styles: email clients strip scripts and external
CSS. It works the same for replies. Send a first one to your own address and check how
it renders. Markdown is not supported.

## Tags

Put tags on the conversation with `--tag` (repeatable), for new emails and replies alike.
Tags that do not exist yet are created.

```bash
./send_email_through_bptickets.py --to jane@example.org --subject "Final notice" \
    --body-file msg.txt --tag cleanup --tag final_notice --send

./send_email_through_bptickets.py --reply-to 889 --body-file reply.txt --tag cleanup --send
```

In YAML: `tags: [cleanup, final_notice]` (or a comma-separated string). `--tag` on the
command line replaces the YAML `tags`.

The tags are set after the message is sent. A reply keeps the tags the conversation
already has and adds the new ones. If tagging fails, the error says the message was
already sent, with the conversation id; it is not sent again.

## Recipients: TO, CC, BCC

- A new email goes to `--to`. `--cc` and `--bcc` (repeatable, or comma-separated) add
  copies.
- A reply goes to the conversation's customer by default, with no CC. `--to ADDR`
  replaces the TO address and each `--cc ADDR` is added as a copy:

```bash
./send_email_through_bptickets.py --reply-to 889 \
    --to jane@example.org --cc bob@example.org --cc ann@example.org \
    --body-file reply.txt --send
```

```yaml
reply_to: 889
to: jane@example.org
cc: [bob@example.org, ann@example.org]
body: |
  Dear Jane, ...
```

### Reply to everyone: `--reply-all`

With `--reply-to ID --reply-all` the script reads the conversation and works out the
recipients itself:

- **TO:** the customer who sent the latest customer message.
- **CC:** everyone else on the conversation (customers, and the To and CC addresses of
  its messages), without the TO address, the agents' own addresses and any
  `--exclude ADDR`.
- An explicit `--to` replaces the computed TO, and that person is copied instead.
  Explicit `--cc` addresses are added to the computed CC.

```bash
# Dry run: reads the conversation and prints the TO and CC lists, sends nothing
./send_email_through_bptickets.py --reply-to 889 --reply-all --exclude support@nbis.se \
    --body-file reply.txt

# Send
./send_email_through_bptickets.py --reply-to 889 --reply-all --exclude support@nbis.se \
    --body-file reply.txt --send
```

YAML: `reply_all: true` and `exclude: [support@nbis.se]`.

Notes:
- The dry run needs `TICKET_API_KEY`, because it reads the conversation. It sends nothing.
- FreeScout does not tell the script the mailbox's own address, so it can show up in CC.
  Add it with `--exclude`.
- Always check the dry run before `--send`: the recipients are worked out from the
  conversation's messages, and a wrong or missing address is easier to catch there.

## Use a YAML file

Put the message and its settings in a YAML file and pass it with `--config`.

New email, `mail.yaml`:

```yaml
to: jane@example.org
subject: Final notice
status: closed
body: |
  Dear Jane,

  Message text here.

  Best regards,
  BP-ops team
```

```bash
./send_email_through_bptickets.py --config mail.yaml          # dry run
./send_email_through_bptickets.py --config mail.yaml --send   # send
```

Reply, `reply.yaml` (the body is read from a file next to the YAML file):

```yaml
reply_to: 1234
body_file: reply_text.txt
```

```bash
./send_email_through_bptickets.py --config reply.yaml --send
```

Keys:

| Key | Meaning |
|---|---|
| `to` | recipient email (new email); for a reply, replaces the default TO address |
| `cc`, `bcc` | extra CC / BCC addresses (list or comma-separated string) |
| `subject` | subject (new email only) |
| `body` | message text; use a block scalar (`body: \|`) |
| `body_file` | file with the message, relative to the YAML file (alternative to `body`) |
| `html` | `true` if the body is HTML (default `false`) |
| `status` | `active`, `pending` or `closed` |
| `reply_to` | conversation id; reply instead of creating a new conversation |
| `reply_all` | `true` to reply to everyone on the conversation (needs `reply_to`) |
| `exclude` | addresses to leave out of a `reply_all` |
| `tags` | tags to put on the conversation (list or comma-separated string) |
| `assign` | for a reply: assign the conversation to `user_id` afterwards (default `true`) |
| `mailbox_id`, `user_id`, `url` | optional; override the environment variables |

Rules:
- Command-line options override the YAML values, for example
  `--config mail.yaml --subject "Other subject"`.
- The script refuses a YAML file that contains `api_key` or `password`, and
  unknown keys.
- There is no `send` key: sending always requires `--send` on the command line,
  so a YAML file cannot send by accident.

## Use from Python

```python
from send_email_through_bptickets import (
    TicketError, get_settings, reply_to_conversation, send_email,
)

settings = get_settings()   # reads the TICKET_* environment variables
try:
    cid = send_email(settings, "jane@example.org", "Subject", "Body text", status="closed")
    reply_to_conversation(settings, cid, "A follow-up message")
except TicketError as e:
    print("failed:", e)
```

## Options

| Option | Meaning |
|---|---|
| `--config YAML` | read the message and settings from a YAML file |
| `--to EMAIL` | recipient (new email); for a reply, the TO address (default: the customer) |
| `--cc EMAIL` | CC address (repeatable) |
| `--bcc EMAIL` | BCC address (repeatable) |
| `--subject TEXT` | subject (new email) |
| `--reply-to ID` | reply to this conversation instead of creating one |
| `--body-file FILE` | file with the message (default: stdin, unless `--config` gives a body) |
| `--reply-all` | with `--reply-to`: reply to everyone (TO = latest customer, CC = the rest) |
| `--exclude EMAIL` | address to leave out of a `--reply-all` (repeatable) |
| `--tag NAME` | tag to put on the conversation (repeatable) |
| `--no-assign` | for a reply: do not assign the conversation to `TICKET_USER_ID` |
| `--html` | the body is HTML |
| `--status STATUS` | `active`, `pending` or `closed` |
| `--send` | actually send (default: dry run) |

## Troubleshooting

- **`Error: TICKET_API_KEY ... are required to use --send`**: set the
  environment variables above.
- **HTTP 401 or 403**: the API key is wrong or lacks permission.
- **HTTP 404 on a reply**: the conversation id does not exist or is not visible
  to the API key.
- **Sent, but not visible in the mailbox**: check the Closed folder or search for
  the subject. The conversation is assigned to `TICKET_USER_ID`, so it also
  appears under Mine.
