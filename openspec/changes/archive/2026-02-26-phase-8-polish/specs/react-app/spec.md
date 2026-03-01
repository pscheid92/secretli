## MODIFIED Requirements

### Requirement: Responsive mobile navigation
The Layout header SHALL collapse into a hamburger menu on small screens.

#### Scenario: Mobile nav collapsed
- **WHEN** the viewport width is below the `md` breakpoint (768px)
- **THEN** the navigation links SHALL be hidden behind a hamburger menu button

#### Scenario: Mobile nav expanded
- **WHEN** the user taps the hamburger menu button
- **THEN** the navigation links SHALL appear in a dropdown/slide-out panel

### Requirement: Responsive history page
The HistoryPage SHALL use a card layout on mobile instead of a table.

#### Scenario: Table on desktop
- **WHEN** the viewport is `md` or wider
- **THEN** the secret history SHALL be displayed as a table

#### Scenario: Cards on mobile
- **WHEN** the viewport is below `md`
- **THEN** each secret SHALL be displayed as a card with key details stacked vertically
