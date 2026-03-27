# Accounts API Frontend Contract

This document describes the current frontend-facing contract and runtime behavior of the accounts-related API.

It is based on the current OpenAPI contract plus the handler/service behavior in the backend. Where the implementation behaves more specifically than the schema alone, that behavior is called out explicitly.

## Scope

This document covers:

- `GET /api/v1/identity`
- `GET/POST /api/v1/organizations`
- `GET/PUT/DELETE /api/v1/organizations/{id}`
- `GET /api/v1/organizations/{id}/users`
- `PUT/DELETE /api/v1/organizations/{id}/users/{username}`
- `GET/POST /api/v1/users`
- `GET/PUT/DELETE /api/v1/users/{username}`

All endpoints require authentication. Error payloads use the shared shape:

```json
{
  "message": "..."
}
```

## Core Concepts

### Identity vs User

There are two different concepts in this API:

- `Identity`: bootstrap information for the currently authenticated user
- `User`: a persisted backend account record

This distinction matters:

- `/api/v1/identity` may return a synthesized response even when no backend user record exists
- `/api/v1/users` and `/api/v1/users/{username}` only deal with persisted backend users

### Organization scope

The `Organization` API model is intentionally minimal. It represents local backend organizations only and does not include:

- member usernames
- child organization IDs
- expanded nested relationships

If the frontend needs the users of an organization, it must call:

- `GET /api/v1/organizations/{id}/users`

### User organization invariant

A persisted backend user without an organization is not a valid state in this system.

In practice this means:

- every persisted `User` always has `organizationId`
- creating a persisted user without `organizationId` is invalid
- backend flows must not produce or keep a persisted user with no organization
- a person without a backend org/account mapping is represented only through synthesized `Identity` as a `regular` user

### Important identity rule

`/api/v1/identity` resolves `organizationId` with this precedence:

- if the authenticated user has a matching backend account with a local organization, `organizationId` comes from the backend account's organization mapping
- if the authenticated user does not have a backend account, `organizationId` falls back to the JWT org from authentication context

## Models

### Identity

Used only by `GET /api/v1/identity`.

```json
{
  "username": "string",
  "kind": "partner | customer | regular | admin",
  "organizationId": "string"
}
```

Fields:

- `username`: authenticated username
- `kind`: frontend bootstrap classification
- `organizationId`: resolved org identifier

Behavior notes:

- `organizationId` is always present
- `organizationId` is a plain string in the contract
- for local users, it is the backend/local organization ID
- for regular users without a backend account, it falls back to the JWT org ID
- the current implementation produces `regular`, `partner`, or `admin`
- `customer` exists in the schema enum but is not currently derived by the service

### Organization

```json
{
  "id": "uuid",
  "name": "string",
  "description": "string?",
  "kind": "partner | admin",
  "icon": "binary?",
  "company": "string",
  "createdAt": "date-time",
  "updatedAt": "date-time"
}
```

Fields:

- `id`: backend organization ID
- `name`: display name
- `description`: optional
- `kind`: only `partner` or `admin`
- `icon`: present in schema as binary/nullable
- `company`: required
- `createdAt`: required
- `updatedAt`: required

Behavior notes:

- the current API model is flat and minimal
- no usernames or child-org references are returned in this model
- organization `name` is unique within a given `company`
- although `icon` exists in the schema, current inbound/outbound mapping does not actively populate it

### User

Represents a persisted backend account.

```json
{
  "username": "string",
  "email": "email",
  "firstName": "string",
  "lastName": "string",
  "phone": "string",
  "location": "latam | emea | na | apac",
  "title": "string?",
  "bio": "string?",
  "organizationId": "uuid",
  "createdAt": "date-time",
  "updatedAt": "date-time?"
}
```

Fields:

- `username`: backend account username, also used in path params
- `email`: required
- `firstName`: required
- `lastName`: required
- `phone`: required
- `location`: required enum
- `title`: optional
- `bio`: optional
- `organizationId`: backend org reference
- `createdAt`: required
- `updatedAt`: optional in practice

Behavior notes:

- `User` does not carry `kind`
- a persisted `User` without `organizationId` is not possible
- regular users are not exposed as `User` resources unless they exist as backend records
- `/api/v1/users` and `/api/v1/users/{username}` do not synthesize missing users

### OrganizationCreate

```json
{
  "name": "string",
  "description": "string",
  "kind": "partner | admin",
  "icon": "binary?",
  "company": "string"
}
```

Required by contract:

- `name`
- `description`
- `kind`
- `company`

Optional:

- `icon`

### OrganizationUpdate

```json
{
  "name": "string?",
  "description": "string?",
  "icon": "binary?",
  "company": "string?"
}
```

All fields are optional.

Behavior note:

- organization `name` must remain unique within its `company`

### UserCreate

```json
{
  "username": "string",
  "email": "email",
  "firstName": "string",
  "lastName": "string",
  "phone": "string",
  "location": "latam | emea | na | apac",
  "title": "string?",
  "bio": "string?",
  "organizationId": "uuid"
}
```

Required:

- `username`
- `email`
- `firstName`
- `lastName`
- `phone`
- `location`

Optional:

- `title`
- `bio`

### UserUpdate

```json
{
  "email": "email?",
  "firstName": "string?",
  "lastName": "string?",
  "phone": "string?",
  "location": "latam | emea | na | apac?",
  "title": "string?",
  "bio": "string?"
}
```

All fields are optional.

## Endpoint Behavior

### `GET /api/v1/identity`

Purpose:

- bootstrap the authenticated user for the UI

Response:

- `200` with `Identity`
- `401` if unauthenticated
- `500` on unexpected backend error

Exact behavior:

- if a backend account exists for the authenticated username:
  - `username` comes from the backend account
  - `organizationId` comes from the backend account's local organization mapping
  - `kind` is derived from the local organization kind
  - currently:
    - admin org => `admin`
    - partner org => `partner`
    - no mapped local org => `regular`
- if no backend account exists:
  - the backend synthesizes an identity response
  - `kind = regular`
  - `organizationId` falls back to the JWT org if present

Example regular user:

```json
{
  "username": "alice",
  "kind": "regular",
  "organizationId": "jwt-org-id"
}
```

Example local partner/admin user:

```json
{
  "username": "bob",
  "kind": "partner",
  "organizationId": "0b67d1c0-1771-4f87-b71d-a0c7e28f5f41"
}
```

Important current limitation:

- although the enum includes `customer`, the service does not currently derive that value

### `GET /api/v1/organizations`

Purpose:

- list organizations

Query params:

- `kind`: optional, `partner | admin`
- `name`: optional case-insensitive partial match
- `company`: optional case-insensitive partial match

Response:

- `200` with `Organization[]`
- `401`
- `500`

### `POST /api/v1/organizations`

Purpose:

- create an organization

Body:

- `OrganizationCreate`

Response:

- `201` with created `Organization`
- `400` for empty body or duplicate-like organization creation failure
- `401`
- `500`

Important note:

- duplicate org creation is surfaced as `400`, not `409`

### `GET /api/v1/organizations/{id}`

Purpose:

- fetch one organization by ID

Response:

- `200` with `Organization`
- `401`
- `404` if the org does not exist
- `500`

Important note:

- the returned model contains only organization metadata
- it does not include members or children

### `PUT /api/v1/organizations/{id}`

Purpose:

- update organization metadata

Body:

- `OrganizationUpdate`

Response:

- `200` with updated `Organization`
- `400` for empty body
- `401`
- `404` if the org does not exist
- `500`

### `DELETE /api/v1/organizations/{id}`

Purpose:

- delete an organization

Response:

- `200` with the deleted `Organization`
- `401`
- `404` if the org does not exist
- `500`

### `GET /api/v1/organizations/{id}/users`

Purpose:

- list backend users belonging to the organization

Response:

- `200` with `User[]`
- `401`
- `404` if the org does not exist
- `500`

### `PUT /api/v1/organizations/{id}/users/{username}`

Purpose:

- assign a backend user to an organization

Response:

- `200` with no response body
- `401`
- `404` if either the org or user does not exist
- `500`

Behavior:

- the backend verifies the org exists
- the backend verifies the user exists
- the user's `organizationId` is set to the provided org ID

### `DELETE /api/v1/organizations/{id}/users/{username}`

Purpose:

- remove a backend user's membership from the specified organization

Response:

- `200` with no response body
- `400` if the user is not currently a member of the org in the path
- `401`
- `404` if the org or user does not exist
- `500`

Behavior:

- the backend validates the org exists
- the backend validates the user exists
- the backend checks that the user's current `organizationId` matches the path org ID
- only then is membership removed

### `GET /api/v1/users`

Purpose:

- list persisted backend users

Query params:

- `organizationId`: UUID
- `location`: optional enum `latam | emea | na | apac`

Response:

- `200` with `User[]`
- `401`
- `500`

Important note:

- this endpoint lists backend user records only
- it does not synthesize regular users

### `POST /api/v1/users`

Purpose:

- create a backend user

Body:

- `UserCreate`

Response:

- `201` with created `User`
- `400` for empty body or missing `organizationId`
- `401`
- `409` if the username already exists
- `500`

Important note:

- persisted backend users always require `organizationId`

### `GET /api/v1/users/{username}`

Purpose:

- fetch a persisted backend user by username

Response:

- `200` with `User`
- `401`
- `404` if the user does not exist
- `500`

Important note:

- unlike `/api/v1/identity`, this endpoint does not synthesize regular users

### `PUT /api/v1/users/{username}`

Purpose:

- update a persisted backend user

Body:

- `UserUpdate`

Response:

- `200` with updated `User`
- `400` for empty body
- `401`
- `404` if the original path user does not exist
- `500`

Behavior:

- the backend first resolves the existing user by path `username`
- then applies the update payload
- `username` is not updateable through this endpoint

### `DELETE /api/v1/users/{username}`

Purpose:

- delete a persisted backend user

Response:

- `200` with the deleted `User`
- `401`
- `404` if the user does not exist
- `500`

## Frontend Guidance

### Use `/api/v1/identity` for session bootstrap

Frontend code should use `/api/v1/identity` to decide:

- who is logged in
- what `kind` the backend considers them
- which `organizationId` should scope the session

Do not use `/api/v1/users/{username}` as a substitute for this purpose.

### Use `/api/v1/users*` only for persisted account management

Frontend code should treat `/api/v1/users` and `/api/v1/users/{username}` as account-management endpoints for stored backend users only.

### Treat `customer` as reserved for now

The contract includes `customer` in the identity enum, but the current backend implementation does not yet derive it. Frontend code should not assume it will be returned until that logic is implemented.

## Quick Reference

### Best endpoint for UI bootstrap

- `GET /api/v1/identity`

### Best endpoint for org member table

- `GET /api/v1/organizations/{id}/users`

### Best endpoint for listing backend-managed users

- `GET /api/v1/users`

### Best endpoint for updating org membership

- `PUT /api/v1/organizations/{id}/users/{username}`
- `DELETE /api/v1/organizations/{id}/users/{username}`
