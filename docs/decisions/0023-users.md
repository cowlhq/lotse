# 0023: Users are invited, and may be kept to one customer

Date: 2026-10-16

## Decision

- A user has a role and an access. The role is `admin`, who may change
  things, or `viewer`, who only looks. The access is the whole hub, or one
  customer, project or environment with everything below it, as an API
  token's scope (decision 0019).
- A user kept to a scope sees only what is in it: in every list, the
  search, alerts and their history, the change history and live updates.
  Anything outside it answers 404, as in the read API (decision 0020).
  Probes are shared; such a user sees their names in check results, not
  their settings.
- An admin kept to a scope may change everything in it: its settings,
  rules, channels and maintenance windows, its hosts and checks, adding
  agents and updating them. What belongs to the whole hub stays with admins
  of the whole hub: users, API tokens, the mail server, probes, the hub's
  update, and the global settings.
- So that a scope's admin stays in it: a channel's secret may refer to the
  hub's environment or secret files only for admins and tokens of the whole
  hub, since those hold every customer's secrets, and a channel that has
  such a reference is pointed elsewhere only by them; what a server answered
  is left out of a channel's errors shown to someone kept to a scope; and
  enrolling a machine again takes over its host only in the host's own
  environment, so no enrollment token takes a machine from another.
- Users join by invitation. An admin of the whole hub gives an email
  address, a role and an access; the hub makes a link that works once and
  for seven days. It is shown to copy and, if the hub has a mail server,
  sent to the address. The person opening it chooses a name and a password
  and is logged in.
- A forgotten password goes the same way: an admin of the whole hub makes a
  new link for the user. Setting a password through a link ends every other
  session of that user. There is no "forgot password" form, so nothing
  depends on a mail server and nobody can find out who has an account.
- Everyone can change their own name, and their password with the current
  one.
- A user who has logged in is never deleted, only disabled, so the history
  keeps who did what; a disabled user's sessions end at once. An
  invitation nobody accepted yet can be withdrawn.
- There is always at least one admin of the whole hub who is not disabled:
  the hub refuses to disable, demote or limit the last one. Nobody changes
  their own role, access, or disables themselves.
- Changes to users are recorded in the hub's history, like any other.

## Why

- lotse is for machines across customers. A customer's people, or someone
  who looks after one project, should see their part without seeing the
  rest; and a viewer role is what a colleague on call or a manager needs.
- One scope per user, like API tokens, keeps the rule to check simple; a
  person for several customers can have the whole hub as viewer, and more
  scopes per user can come later.
- A link the admin can copy works on every hub, with or without mail; the
  admin never knows the password.
