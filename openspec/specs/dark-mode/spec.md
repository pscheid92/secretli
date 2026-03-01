## Requirements

### Requirement: System preference detection
The application SHALL detect the user's system color scheme preference on initial load.

#### Scenario: System prefers dark
- **WHEN** the user's OS is set to dark mode and no manual preference is stored
- **THEN** the application SHALL render in dark mode

#### Scenario: System prefers light
- **WHEN** the user's OS is set to light mode and no manual preference is stored
- **THEN** the application SHALL render in light mode

### Requirement: Manual theme toggle
The application SHALL provide a toggle in the header to switch between light, dark, and system modes.

#### Scenario: Toggle to dark
- **WHEN** the user selects dark mode from the toggle
- **THEN** the application SHALL switch to dark mode and persist the choice in localStorage

#### Scenario: Toggle to system
- **WHEN** the user selects system mode
- **THEN** the application SHALL follow the OS preference and clear the manual override from localStorage

### Requirement: Dark mode styling
All UI components SHALL have appropriate dark mode styles.

#### Scenario: Background colors
- **WHEN** dark mode is active
- **THEN** page backgrounds SHALL use dark colors (e.g., gray-900/950) and card backgrounds SHALL use slightly lighter dark colors (e.g., gray-800)

#### Scenario: Text colors
- **WHEN** dark mode is active
- **THEN** primary text SHALL be light (e.g., gray-100) and secondary text SHALL be gray-400

#### Scenario: Form inputs
- **WHEN** dark mode is active
- **THEN** form inputs SHALL have dark backgrounds, light text, and visible borders

### Requirement: Theme persistence
The user's theme preference SHALL persist across page reloads and sessions.

#### Scenario: Preference survives reload
- **WHEN** a user sets dark mode and reloads the page
- **THEN** the application SHALL still be in dark mode
