### Requirement: Brand identity
Syntopica SHALL have consistent brand identity across all touchpoints including product name, repo name, module names, and Docker naming.

#### Scenario: Product name used in README
- **WHEN** a user visits the GitHub repository
- **THEN** the README SHALL display "Syntopica" as the product name

#### Scenario: Go module name is syntopica-backend
- **WHEN** the Go module is imported
- **THEN** the module name SHALL be `syntopica-backend`

#### Scenario: NPM package name is @syntopica/web
- **WHEN** the frontend package is installed
- **THEN** the package name SHALL be `@syntopica/web`

#### Scenario: Docker services use syntopica- prefix
- **WHEN** Docker services are listed
- **THEN** service names SHALL use `syntopica-*` prefix

#### Scenario: Default database name is syntopica
- **WHEN** the database is created by docker-compose
- **THEN** the default database name SHALL be `syntopica`

### Requirement: Tagline
Syntopica SHALL have a consistent tagline used in README and branding materials.

#### Scenario: Tagline displayed in README
- **WHEN** a user reads the README
- **THEN** the tagline "Where feeds become topics" SHALL be displayed
