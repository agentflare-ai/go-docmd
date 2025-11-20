# Contributing to go-docmd

Thank you for your interest in contributing to go-docmd! We welcome contributions from the community.

## Development Setup

1. **Prerequisites**: Go 1.24.5 or later
2. **Clone the repository**:
   ```bash
   git clone https://github.com/agentflare-ai/go-docmd.git
   cd go-docmd
   ```
3. **Install dependencies**:
   ```bash
   go mod download
   ```
4. **Run tests**:
   ```bash
   go test ./...
   ```

## Code Style

- Follow standard Go formatting (`go fmt`)
- Use `goimports` for import organization
- Follow Go naming conventions
- Add documentation comments for exported functions/types
- Write tests for new functionality

## Pull Request Process

1. **Fork** the repository
2. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```
3. **Make your changes** with tests
4. **Run tests** and ensure they pass:
   ```bash
   go test ./...
   ```
5. **Update documentation** if needed
6. **Commit your changes** with descriptive commit messages
7. **Push to your fork** and **create a pull request**

## Commit Messages

Use conventional commit format:
- `feat:` for new features
- `fix:` for bug fixes
- `docs:` for documentation changes
- `test:` for test additions/changes
- `chore:` for maintenance tasks

## Testing

- All new code should include tests
- Run `go test ./...` before submitting
- Tests should cover both happy path and error cases
- Use table-driven tests where appropriate

## Issues

- Check existing issues before creating new ones
- Use issue templates when available
- Provide clear reproduction steps for bugs

## License

By contributing to this project, you agree that your contributions will be licensed under the MIT License.
