# 0010: People are invited, with one or more accesses

Date: 2026-10-25

Taken from lotse 0.1, decisions 0023 and 0025, with spaces in place of
scopes.

## Decision

- The person who sets up the hub owns it. Others are invited by a link
  sent by email.
- A person has one or more **accesses**. An access is the whole hub or one
  space with its sub-spaces, and a role for it: *admin*, who may change
  things there, or *viewer*, who only looks. Someone can be admin of
  "Sports club" and viewer of "Parents".
- Accesses may lie inside one another only to give more: viewer of the
  whole hub and admin of one space is a pair; anything else adds nothing
  and is refused. A space appears once in a person's accesses.
- Someone without access to the whole hub sees only their spaces: the home
  page, the breadcrumb, search and alerts show nothing else, not even that
  there is more. Checks in no space belong to the hub, and are seen only
  with access to it.
- What belongs to the whole hub (people, channels of the hub, the mail
  server, the hub's settings) stays with admins of the whole hub. There is
  always at least one who can sign in.
- A space that is anyone's access cannot be deleted.
- Sign-in is by password; passkeys may come later.

## Why

People who look after others' things are responsible for one, on call for
another, and should not see a third. A role per access says that plainly,
without an order or a precedence to learn.
