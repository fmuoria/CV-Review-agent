# Contributing to CV Review Agent

Thank you for your interest in contributing to CV Review Agent! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/CV-Review-agent.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Test your changes
6. Commit and push
7. Create a Pull Request

## Development Setup

See [SETUP.md](SETUP.md) for detailed setup instructions.

Quick start:
```bash
# Install dependencies
make deps

# Build the project
make build

# Run tests
make test

# Format code
make fmt

# Run linter
make vet
```

## Code Style

- Follow standard Go conventions and idioms
- Run `go fmt` before committing
- Run `go vet` to catch common issues
- Write meaningful commit messages
- Add comments for complex logic

## Testing

- Write tests for new functionality
- Ensure all tests pass before submitting PR
- Aim for good test coverage
- Run `go test -v ./...` to run all tests

## Pull Request Process

1. Update documentation if needed
2. Add tests for new features
3. Ensure all tests pass
4. Update README.md if adding new features
5. Request review from maintainers

## Commit Message Guidelines

- Use present tense ("Add feature" not "Added feature")
- Use imperative mood ("Move cursor to..." not "Moves cursor to...")
- Limit first line to 72 characters
- Reference issues and PRs when relevant

Examples:
```
Add support for PDF parsing
Fix bug in CV matching logic
Update README with new examples
Refactor scoring module for clarity
```

## Code Review Process

- All submissions require review
- Maintainers will review your PR
- Address feedback promptly
- Keep PR scope focused

## Bug Reports

When filing a bug report, include:
- Go version
- Operating system
- Steps to reproduce
- Expected behavior
- Actual behavior
- Relevant logs or error messages

## Feature Requests

When requesting a feature:
- Describe the use case
- Explain why existing features don't work
- Provide examples if possible
- Consider contributing the feature yourself!

## Questions?

- Open an issue with the "question" label
- Check existing issues first
- Be respectful and patient

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help create a welcoming environment
- Report unacceptable behavior to maintainers

Thank you for contributing! 🎉
