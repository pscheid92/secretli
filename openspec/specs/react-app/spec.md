## ADDED Requirements

### Requirement: React app with TypeScript and Vite
The frontend SHALL be a React 19 application using TypeScript 5 and Vite 6 as the build tool. The production build output SHALL be placed in `web/frontend/dist/`.

#### Scenario: Production build succeeds
- **WHEN** `npm run build` is run in `web/frontend/`
- **THEN** the build succeeds without errors
- **AND** output is written to `web/frontend/dist/`

### Requirement: Tailwind CSS 4 integration
The frontend SHALL use Tailwind CSS 4 for styling, integrated via the Vite plugin.

#### Scenario: Tailwind classes render correctly
- **WHEN** a component uses Tailwind utility classes (e.g., `className="text-lg font-bold"`)
- **THEN** the corresponding styles are applied in the rendered output

### Requirement: React Router with all placeholder routes
The frontend SHALL use React Router 7 with the following routes, each rendering a placeholder page component:

| Path | Component | Description |
|---|---|---|
| `/` | SharePage | Create a text secret |
| `/s` | RetrievePage | Retrieve a secret (reads hash fragment) |
| `/file` | FilePage | Upload encrypted file |
| `*` | NotFoundPage | 404 page |

#### Scenario: Share page at root
- **WHEN** the user navigates to `/`
- **THEN** the SharePage component renders

#### Scenario: Retrieve page with hash fragment
- **WHEN** the user navigates to `/s#someShareSecret`
- **THEN** the RetrievePage component renders

#### Scenario: Unknown path shows 404
- **WHEN** the user navigates to `/nonexistent`
- **THEN** the NotFoundPage component renders

### Requirement: Layout component with navigation
The frontend SHALL have a Layout component that wraps all pages with a navigation header and footer. The navigation SHALL include links to Share (`/`) and File (`/file`).

#### Scenario: Navigation links
- **WHEN** any page renders
- **THEN** the Layout shows links to Share and File

### Requirement: TanStack Query provider
The frontend SHALL set up a TanStack Query (React Query v5) `QueryClientProvider` at the app root for use by subsequent phases.

#### Scenario: QueryClient is available
- **WHEN** any component renders inside the app
- **THEN** a `QueryClient` is available via React Query's context

