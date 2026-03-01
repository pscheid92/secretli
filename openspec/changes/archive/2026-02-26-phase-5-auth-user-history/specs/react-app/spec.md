## MODIFIED Requirements

### Requirement: Layout component with navigation
The frontend SHALL have a Layout component that wraps all pages with a navigation header and footer. The navigation SHALL include links to Share (`/`), File (`/file`), and conditionally show Login (`/login`) or the user's display name with a History link (`/history`) and Logout button based on authentication state.

#### Scenario: Navigation links for unauthenticated user
- **WHEN** no user is logged in
- **THEN** the Layout shows links to Share, File, and Login

#### Scenario: Navigation links for authenticated user
- **WHEN** a user is logged in
- **THEN** the Layout shows links to Share, File, History, the user's display name, and a Logout button
- **AND** the Login link is not shown

## ADDED Requirements

### Requirement: AuthContext provider
The frontend SHALL provide an `AuthContext` that exposes the current user, `login()`, `register()`, `logout()` functions, and `isLoading` state. The context SHALL use TanStack Query internally: `useQuery` for `GET /api/v1/auth/me` on mount and `useMutation` for login/register/logout.

#### Scenario: User loaded on app mount
- **WHEN** the app mounts and a valid session cookie exists
- **THEN** the AuthContext fetches `/api/v1/auth/me` and provides the user object

#### Scenario: No session on mount
- **WHEN** the app mounts without a session cookie
- **THEN** the AuthContext provides `user: null` and `isLoading: false`

#### Scenario: Login updates context
- **WHEN** `login(email, password)` succeeds
- **THEN** the user object is updated in the context

#### Scenario: Logout clears context
- **WHEN** `logout()` is called
- **THEN** the user is set to null in the context

### Requirement: LoginPage
The frontend SHALL render a login form at `/login` with email and password fields. On successful login, the page SHALL redirect to `/`. On error, the page SHALL display the error message. If already logged in, the page SHALL redirect to `/`.

#### Scenario: Successful login
- **WHEN** the user submits valid credentials
- **THEN** the user is logged in and redirected to `/`

#### Scenario: Invalid credentials
- **WHEN** the user submits invalid credentials
- **THEN** an error message is displayed

#### Scenario: Already logged in
- **WHEN** an authenticated user visits `/login`
- **THEN** they are redirected to `/`

### Requirement: RegisterPage
The frontend SHALL render a registration form at `/register` with email, password, and display name fields. On successful registration, the page SHALL redirect to `/`. On error (duplicate email, validation), the page SHALL display the error message.

#### Scenario: Successful registration
- **WHEN** the user submits valid registration details
- **THEN** the user is registered, logged in, and redirected to `/`

#### Scenario: Duplicate email
- **WHEN** the user submits an already-registered email
- **THEN** an error message is displayed

#### Scenario: Already logged in
- **WHEN** an authenticated user visits `/register`
- **THEN** they are redirected to `/`

### Requirement: HistoryPage
The frontend SHALL render a paginated list of the user's created secrets at `/history`. Each entry SHALL show the label (or "Untitled"), secret type, creation date, expiration date, and retrieval status. If not authenticated, the page SHALL redirect to `/login`.

#### Scenario: Authenticated user with secrets
- **WHEN** an authenticated user visits `/history`
- **THEN** their secrets are listed with metadata

#### Scenario: Unauthenticated user
- **WHEN** an unauthenticated user visits `/history`
- **THEN** they are redirected to `/login`

#### Scenario: Empty history
- **WHEN** an authenticated user has no secrets
- **THEN** a message like "No secrets yet" is displayed

#### Scenario: Pagination
- **WHEN** the user has more than 20 secrets
- **THEN** pagination controls are shown to navigate between pages
