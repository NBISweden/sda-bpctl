#!/usr/bin/env python3
"""
Send an email, or reply to an existing conversation, through the NBIS ticketing
system (FreeScout, https://tickets.nbis.se) using its REST API.

New email:  created as a new conversation in a mailbox, with a message thread from an
            agent, which FreeScout delivers to the customer by email. The conversation
            stays in the mailbox, so there is a record of what was sent.
Reply:      with --reply-to ID, the message is added as an agent reply to the existing
            conversation ID (number in its URL) and emailed to its customer.

Settings come from the environment (never from arguments or the YAML file for the key):
  TICKET_API_KEY     FreeScout API key (TICKET_PASSWORD is accepted as an alias)
  TICKET_MAILBOX_ID  id of the mailbox the email is sent from (number in the mailbox URL)
  TICKET_USER_ID     id of the agent the message is sent as (number in /users/profile/<id>)
  TICKET_URL         (optional) default https://tickets.nbis.se
mailbox_id, user_id and url may also be given in the YAML file; the environment wins
only when the YAML file does not set them. The API key is never read from YAML.

Everything else can be given in a YAML file with --config (command-line options
override it). Keys:
  to:          recipient email (new email; for a reply, overrides the default TO address)
  cc, bcc:     extra CC / BCC addresses (a list, or a comma-separated string)
  subject:     subject (new email only)
  body:        message text (use a block scalar:  body: |)
  body_file:   file with the message, relative to the YAML file (alternative to body)
  html:        true if the body is HTML (default false: plain text)
  status:      active | pending | closed
  reply_to:    conversation id; reply instead of creating a new conversation
  assign:      for a reply: assign the conversation to user_id after replying (default true)
  reply_all:   true to reply to everyone on the conversation (see Recipients)
  exclude:     addresses to leave out of a reply_all (e.g. the mailbox's own address)
  tags:        tag(s) to put on the conversation (a list, or a comma-separated string)
  mailbox_id, user_id, url:   optional, see above

Recipients: a new email goes to --to, with optional --cc/--bcc. A reply goes to the
conversation's customer unless --to is given; --to ADDR sets the TO address and every
--cc ADDR (repeatable) is put in CC, e.g. to reply to one person and copy the others.

--reply-all (with --reply-to) works the recipients out from the conversation: TO is the
sender of the latest customer message, CC is everyone else who appears on the conversation
(customers, To and CC addresses of its messages), minus the TO address, the agents' own
addresses and any --exclude ADDR (add the mailbox's own address here if it shows up in
CC). An explicit --to replaces the computed TO; explicit --cc addresses are added to the
computed CC. The dry run prints the result (it reads the conversation, nothing is sent).

Assignee: after a reply the conversation is assigned to the sending user (TICKET_USER_ID);
use --no-assign (or `assign: false`) to leave the assignee as it is. New emails are always
assigned to the sending user.

Tags (--tag NAME, repeatable, or `tags:` in YAML): applied to the conversation after the
message is sent, for both new emails and replies. Tags that do not exist yet are created.
A reply keeps the tags the conversation already has and adds the new ones.

Nothing is sent unless --send is given; the default is a dry run that prints the message.

Examples:
  ./send_email_through_bptickets.py --to a@b.se --subject "Hello" --body-file msg.txt --send
  ./send_email_through_bptickets.py --reply-to 1234 --body-file reply.txt --send
  ./send_email_through_bptickets.py --config mail.yaml --send
  ./send_email_through_bptickets.py --reply-to 889 --to jane@x.org --cc bob@x.org --cc ann@x.org \
      --body-file r.txt --send
  ./send_email_through_bptickets.py --reply-to 889 --body-file r.txt --tag cleanup --tag final_notice --send

Example mail.yaml:
  to: a@b.se
  subject: Hello
  status: closed
  tags: [cleanup, final_notice]
  body: |
    Dear Jane,

    This is a test message.

    Best regards,
    BP-ops team

Library:
  from send_email_through_bptickets import get_settings, send_email, reply_to_conversation
"""

import argparse
import html
import json
import os
import sys
import urllib.error
import urllib.request

DEFAULT_URL = "https://tickets.nbis.se"
STATUSES = ("active", "pending", "closed")
YAML_KEYS = {"reply_all", "exclude", "to", "cc", "bcc", "subject", "body", "body_file", "html", "status", "reply_to", "tags",
             "assign", "mailbox_id", "user_id", "url"}


class TicketError(Exception):
    """Raised when the ticketing system rejects or fails a request."""


def normalize_list(value):
    """Turn None, a comma-separated string or a list into a de-duplicated list of names
    (tags or email addresses)."""
    if not value:
        return []
    if isinstance(value, str):
        value = value.split(",")
    out, seen = [], set()
    for t in value:
        t = str(t).strip()
        if t and t.casefold() not in seen:
            seen.add(t.casefold())
            out.append(t)
    return out


def get_settings(require_send=True, overrides=None):
    """Read the TICKET_* environment variables; `overrides` (e.g. from YAML) take
    precedence for mailbox_id, user_id and base_url. Exit with a message if incomplete."""
    overrides = overrides or {}
    s = {
        "api_key": os.environ.get("TICKET_API_KEY") or os.environ.get("TICKET_PASSWORD", ""),
        "mailbox_id": str(overrides.get("mailbox_id") or os.environ.get("TICKET_MAILBOX_ID", "")),
        "user_id": str(overrides.get("user_id") or os.environ.get("TICKET_USER_ID", "")),
        "base_url": overrides.get("url") or os.environ.get("TICKET_URL", DEFAULT_URL),
    }
    if require_send and not (s["api_key"] and s["mailbox_id"].isdigit() and s["user_id"].isdigit()):
        sys.exit("Error: TICKET_API_KEY (environment) and the numeric mailbox id and user id "
                 "(TICKET_MAILBOX_ID / TICKET_USER_ID or mailbox_id / user_id in the YAML file) "
                 "are required to use --send.")
    return s


def text_to_html(body):
    """Escape plain text and keep its line breaks."""
    return html.escape(body).replace("\n", "<br>\n")


def _request(settings, method, path, payload=None):
    """Call the FreeScout API. Returns (response headers, response body bytes)."""
    req = urllib.request.Request(
        f"{settings['base_url'].rstrip('/')}{path}",
        data=None if payload is None else json.dumps(payload).encode(),
        headers={"Content-Type": "application/json", "Accept": "application/json",
                 "X-FreeScout-API-Key": settings["api_key"]},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return resp.headers, resp.read()
    except urllib.error.HTTPError as e:
        raise TicketError(f"HTTP {e.code} {e.reason}: {e.read().decode(errors='replace')[:300]!r}") from e
    except urllib.error.URLError as e:
        raise TicketError(str(e)) from e


def reply_all_recipients(settings, conversation_id, exclude=()):
    """Work out (to, cc) for a reply to everybody on a conversation.

    TO: the customer who sent the latest customer message (falling back to the
    conversation's customer). CC: every other address seen on the conversation (customers,
    To and CC of its messages), without TO, the agents' own addresses and `exclude`.
    Raises TicketError."""
    cid = int(conversation_id)
    _, raw = _request(settings, "GET", f"/api/conversations/{cid}?embed=threads")
    try:
        conv = json.loads(raw)
    except ValueError as e:
        raise TicketError(f"cannot read conversation #{cid}") from e
    threads = [t for t in (conv.get("_embedded") or {}).get("threads", []) if isinstance(t, dict)]
    threads.sort(key=lambda t: t.get("createdAt") or "")

    def email_of(obj):
        return (obj.get("email") or "").strip() if isinstance(obj, dict) else ""

    skip = {a.casefold() for a in normalize_list(list(exclude))}
    for t in threads:  # agents' own addresses
        if t.get("type") == "message" or (t.get("createdBy") or {}).get("type") == "user":
            skip.add(email_of(t.get("createdBy")).casefold())

    to = ""
    for t in reversed(threads):
        if t.get("type") == "customer" and email_of(t.get("customer")):
            to = email_of(t["customer"])
            break
    to = to or email_of(conv.get("customer"))
    if not to:
        raise TicketError(f"no customer address found on conversation #{cid}")

    seen = {to.casefold(), ""} | skip
    cc = []
    candidates = [email_of(conv.get("customer"))] + list(normalize_list(conv.get("cc")))
    for t in threads:
        if t.get("type") == "note":
            continue
        candidates += [email_of(t.get("customer"))]
        candidates += normalize_list(t.get("to")) + normalize_list(t.get("cc"))
    for addr in candidates:
        if addr.casefold() not in seen:
            seen.add(addr.casefold())
            cc.append(addr)
    return [to], cc


def add_tags(settings, conversation_id, tags, keep_existing=True):
    """Add tags to a conversation (missing tags are created by FreeScout).

    FreeScout's tags endpoint replaces the whole tag list, so with keep_existing the
    conversation's current tags are read first and kept. Raises TicketError."""
    tags = normalize_list(tags)
    if not tags:
        return
    cid = int(conversation_id)
    if keep_existing:
        _, raw = _request(settings, "GET", f"/api/conversations/{cid}?embed=tags")
        try:
            data = json.loads(raw)
        except ValueError as e:
            raise TicketError(f"cannot read current tags of conversation #{cid}") from e
        current = data.get("tags") or (data.get("_embedded") or {}).get("tags") or []
        tags = normalize_list([t.get("name", "") if isinstance(t, dict) else t for t in current] + tags)
    _request(settings, "PUT", f"/api/conversations/{cid}/tags", {"tags": tags})


def _tag_after_send(settings, conversation_id, tags, keep_existing):
    try:
        add_tags(settings, conversation_id, tags, keep_existing=keep_existing)
    except TicketError as e:
        raise TicketError(f"the message was sent (conversation #{conversation_id}) "
                          f"but adding tags failed: {e}") from e


def send_email(settings, to_addr, subject, body, status="active", is_html=False, tags=None,
               cc=None, bcc=None):
    """Send one email as a new conversation. Returns the new conversation id.

    body is plain text unless is_html is True. tags (optional) are put on the new
    conversation. Raises TicketError on failure.
    """
    user_id = int(settings["user_id"])
    thread = {"type": "message", "text": body if is_html else text_to_html(body),
              "user": user_id, "customer": {"email": to_addr}}
    if normalize_list(cc):
        thread["cc"] = normalize_list(cc)
    if normalize_list(bcc):
        thread["bcc"] = normalize_list(bcc)
    payload = {
        "type": "email",
        "mailboxId": int(settings["mailbox_id"]),
        "status": status,  # active | pending | closed
        "assignTo": user_id,  # keeps it visible under "Mine" for the sender
        "subject": subject,
        "customer": {"email": to_addr},
        "threads": [thread],
    }
    # 201 Created, the conversation id is in the X-FreeScout-Conversation-Id header
    headers, _ = _request(settings, "POST", "/api/conversations", payload)
    cid = headers.get("X-FreeScout-Conversation-Id")
    if normalize_list(tags):
        if not cid:
            raise TicketError("the message was sent but the conversation id is unknown, "
                              "so tags were not added")
        _tag_after_send(settings, cid, tags, keep_existing=False)
    return cid


def assign_conversation(settings, conversation_id, user_id=None):
    """Assign a conversation to an agent (default: the sending user). Raises TicketError."""
    uid = int(user_id if user_id is not None else settings["user_id"])
    _request(settings, "PUT", f"/api/conversations/{int(conversation_id)}",
             {"assignTo": uid, "byUser": uid})


def reply_to_conversation(settings, conversation_id, body, status=None, is_html=False, tags=None,
                          to=None, cc=None, bcc=None, assign=True):
    """Add an agent reply to an existing conversation; FreeScout emails it to the
    customer (or to `to`, with `cc` / `bcc` copies). status=None leaves the conversation
    status unchanged. With assign (default) the conversation is then assigned to the
    sending user (TICKET_USER_ID). tags (optional) are added to the conversation's
    existing tags. Raises TicketError."""
    payload = {
        "type": "message",
        "text": body if is_html else text_to_html(body),
        "user": int(settings["user_id"]),
    }
    if status:
        payload["status"] = status
    for key, value in (("to", to), ("cc", cc), ("bcc", bcc)):
        if normalize_list(value):
            payload[key] = normalize_list(value)
    _request(settings, "POST", f"/api/conversations/{int(conversation_id)}/threads", payload)
    if assign:
        try:
            assign_conversation(settings, conversation_id)
        except TicketError as e:
            raise TicketError(f"the message was sent (conversation #{conversation_id}) "
                              f"but assigning it to user {settings['user_id']} failed: {e}") from e
    if normalize_list(tags):
        _tag_after_send(settings, conversation_id, tags, keep_existing=True)
    return conversation_id


def load_config(path):
    """Load and validate the YAML file. Returns a dict with body_file already resolved."""
    try:
        import yaml
    except ImportError:
        sys.exit("Error: --config needs PyYAML (pip install pyyaml).")
    if not os.path.isfile(path):
        sys.exit(f"Error: config file not found: {path}")
    try:
        with open(path, encoding="utf-8") as fh:
            cfg = yaml.safe_load(fh) or {}
    except (OSError, UnicodeDecodeError) as e:
        sys.exit(f"Error: cannot read config file {path}: {e}")
    except yaml.YAMLError as e:
        sys.exit(f"Error: invalid YAML in {path}: {e}")
    if not isinstance(cfg, dict):
        sys.exit(f"Error: {path} must contain a YAML mapping of keys.")
    if "api_key" in cfg or "password" in cfg:
        sys.exit("Error: do not put the API key in the YAML file; use TICKET_API_KEY.")
    unknown = set(cfg) - YAML_KEYS
    if unknown:
        sys.exit(f"Error: unknown key(s) in {path}: {', '.join(sorted(unknown))}")
    if cfg.get("body_file"):
        cfg["body_file"] = os.path.join(os.path.dirname(os.path.abspath(path)), cfg["body_file"])
    return cfg


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--config", metavar="YAML", help="YAML file with the message and its settings")
    ap.add_argument("--to", metavar="EMAIL", help="recipient email (new email); for a reply, the TO address "
                                            "(default: the conversation's customer)")
    ap.add_argument("--cc", action="append", metavar="EMAIL",
                    help="CC address (repeatable; overrides `cc` in YAML)")
    ap.add_argument("--bcc", action="append", metavar="EMAIL",
                    help="BCC address (repeatable; overrides `bcc` in YAML)")
    ap.add_argument("--reply-all", action="store_true", default=None, dest="reply_all",
                    help="with --reply-to: reply to everyone on the conversation "
                         "(TO = latest customer, CC = everyone else)")
    ap.add_argument("--exclude", action="append", metavar="EMAIL",
                    help="address to leave out of a --reply-all (repeatable)")
    ap.add_argument("--subject", help="subject (new email)")
    ap.add_argument("--reply-to", metavar="ID", type=int,
                    help="reply to this existing conversation id instead of creating a new one")
    ap.add_argument("--body-file", metavar="FILE", help="file with the message (default: stdin)")
    ap.add_argument("--html", action="store_true", default=None, help="body is HTML, not plain text")
    ap.add_argument("--status", choices=STATUSES,
                    help="conversation status (new email: default active; reply: unchanged)")
    ap.add_argument("--tag", action="append", metavar="NAME", dest="tags",
                    help="tag to put on the conversation (repeatable; overrides `tags` in YAML)")
    ap.add_argument("--no-assign", action="store_true", default=None, dest="no_assign",
                    help="for a reply: do not assign the conversation to TICKET_USER_ID")
    ap.add_argument("--send", action="store_true", help="actually send (default: dry run)")
    args = ap.parse_args()

    cfg = load_config(args.config) if args.config else {}
    # command-line options override the YAML file
    for key in ("to", "subject", "reply_to", "html", "status", "tags", "cc", "bcc",
                "reply_all", "exclude"):
        v = getattr(args, key)
        if v is not None:
            cfg[key] = v
    if args.body_file:
        cfg["body_file"] = args.body_file
        cfg.pop("body", None)
    if args.no_assign:
        cfg["assign"] = False

    if cfg.get("body"):
        body = str(cfg["body"])
    elif cfg.get("body_file"):
        if not os.path.isfile(cfg["body_file"]):
            sys.exit(f"Error: body file not found: {cfg['body_file']}")
        try:
            with open(cfg["body_file"], encoding="utf-8") as fh:
                body = fh.read()
        except (OSError, UnicodeDecodeError) as e:
            sys.exit(f"Error: cannot read body file {cfg['body_file']}: {e}")
    elif args.config:
        sys.exit("Error: no body (set body or body_file in the YAML file, or use --body-file).")
    else:
        body = sys.stdin.read()
    if not body.strip():
        sys.exit("Error: empty message body.")

    reply_to = cfg.get("reply_to")
    status = cfg.get("status")
    if status is not None and status not in STATUSES:
        sys.exit(f"Error: status must be one of {', '.join(STATUSES)}.")
    if not reply_to and not (cfg.get("to") and cfg.get("subject")):
        sys.exit("Error: a new email needs `to` and `subject` (or use --reply-to ID to reply).")
    if reply_to and not str(reply_to).isdigit():
        sys.exit("Error: reply_to must be a conversation id (number).")
    is_html = bool(cfg.get("html"))
    tags = normalize_list(cfg.get("tags"))
    to_list = normalize_list(cfg.get("to")) if reply_to else []
    cc, bcc = normalize_list(cfg.get("cc")), normalize_list(cfg.get("bcc"))
    for addr in to_list + cc + bcc:
        if "@" not in addr:
            sys.exit(f"Error: not an email address: {addr!r}")

    reply_all = bool(cfg.get("reply_all"))
    if reply_all and not reply_to:
        sys.exit("Error: --reply-all needs --reply-to ID.")
    settings = None
    if reply_all:
        settings = get_settings(overrides=cfg)  # reading the conversation needs the API key
        try:
            auto_to, auto_cc = reply_all_recipients(settings, reply_to, normalize_list(cfg.get("exclude")))
        except TicketError as e:
            sys.exit(f"FAILED to work out the recipients: {e}")
        to_list = to_list or auto_to
        to_seen = {t.casefold() for t in to_list}
        # if --to replaced the computed TO, that person is copied instead
        cc = [c for c in normalize_list(auto_to + auto_cc + cc) if c.casefold() not in to_seen]

    target = (f"REPLY to conversation #{reply_to}" if reply_to
              else f"NEW to <{cfg['to']}> | {cfg['subject']}")
    if reply_to and to_list:
        target += f" | to: {', '.join(to_list)}"
    if cc:
        target += f" | cc: {', '.join(cc)}"
    if bcc:
        target += f" | bcc: {', '.join(bcc)}"
    if tags:
        target += f" | tags: {', '.join(tags)}"
    if not args.send:
        print(f"--- DRY RUN: {target}\n{body}", file=sys.stderr)
        sys.exit(0)

    settings = settings or get_settings(overrides=cfg)
    try:
        if reply_to:
            reply_to_conversation(settings, reply_to, body, status=status, is_html=is_html,
                                  tags=tags, to=to_list, cc=cc, bcc=bcc,
                                  assign=cfg.get("assign", True))
            print(f"replied to conversation #{reply_to}"
                  + (f", assigned to user {settings['user_id']}" if cfg.get("assign", True) else "")
                  + (f", tags added: {', '.join(tags)}" if tags else ""))
        else:
            cid = send_email(settings, cfg["to"], cfg["subject"], body,
                             status=status or "active", is_html=is_html, tags=tags, cc=cc, bcc=bcc)
            print(f"sent to {cfg['to']}, conversation #{cid}"
                  + (f", tags: {', '.join(tags)}" if tags else ""))
    except TicketError as e:
        sys.exit(f"FAILED: {e}")


if __name__ == "__main__":
    main()
