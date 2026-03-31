# Security Policy

This document outlines the security policy for the **Netllm** project. We take security seriously and are committed to ensuring the safety and integrity of our software and its users.

---

## Supported Versions

The following versions of Netllm are currently being supported with security updates:

| Version | Supported          | Status       |
| ------- | ------------------ | ------------ |
| 1.0.x   | :white_check_mark: | Active       |
| < 1.0   | :x:                | Unsupported  |

> **Note:** We strongly recommend upgrading to the latest supported version to ensure you receive the latest security patches and updates.

---

## Reporting a Vulnerability

We appreciate the efforts of security researchers and the community in helping us maintain a secure project. If you discover a security vulnerability, please follow the guidelines below.

### How to Report

You may report a security vulnerability through any of the following channels:

| Channel          | Contact Information                        |
| ---------------- | ------------------------------------------ |
| **Email**        | security@netllm.dev                        |
| **GitHub Issues**| Create a issue with the `security` label   |

> **Important:** Please do **not** publicly disclose unpatched vulnerabilities. Follow our responsible disclosure process outlined below.

### What to Include

When reporting a vulnerability, please provide as much of the following information as possible:

- **Description** – A clear and concise description of the vulnerability.
- **Impact** – The potential impact of the vulnerability (e.g., data exposure, privilege escalation, denial of service).
- **Affected Versions** – The version(s) of Netllm affected by the vulnerability.
- **Reproduction Steps** – Step-by-step instructions to reproduce the issue.
- **Proof of Concept (PoC)** – A working exploit or code snippet demonstrating the vulnerability (if applicable).
- **Environment** – Details about the environment where the vulnerability was discovered (OS, runtime version, network configuration, etc.).
- **Suggested Fix** – Any recommendations for mitigating or resolving the issue (optional but appreciated).

### Response Time Expectations

We are committed to responding to security reports in a timely manner. Below are our target response times:

| Stage                         | Target Response Time     |
| ----------------------------- | ------------------------ |
| **Acknowledgment**            | Within 48 hours          |
| **Initial Assessment**        | Within 5 business days   |
| **Status Update**             | Every 7 days until resolved |
| **Patch / Fix Delivery**      | Depends on severity (see below) |

#### Severity-Based Response Times

| Severity Level | Description                                      | Target Resolution Time |
| -------------- | ------------------------------------------------ | ---------------------- |
| **Critical**   | Remote code execution, data breach, auth bypass  | Within 72 hours        |
| **High**       | Privilege escalation, significant data leak      | Within 7 days          |
| **Medium**     | Limited information disclosure, DoS              | Within 14 days         |
| **Low**        | Minor info leak, best practice violations        | Within 30 days         |

---

## Security Best Practices

When deploying and operating Netllm, we recommend adhering to the following security best practices:

### Transport Layer Security (TLS)

- **Enable TLS everywhere.** All communication between clients, services, and databases must be encrypted using TLS 1.2 or higher.
- **Use strong cipher suites.** Prefer AEAD ciphers such as AES-GCM or ChaCha20-Poly1305.
- **Rotate certificates regularly.** Use automated certificate management tools (e.g., Let's Encrypt, cert-manager) to keep certificates up to date.
- **Enforce mutual TLS (mTLS)** for internal service-to-service communication where applicable.

### Authentication & Authorization

- **Use strong authentication mechanisms.** Integrate with identity providers (IdPs) using OAuth 2.0 / OpenID Connect where possible.
- **Enforce multi-factor authentication (MFA)** for all administrative and privileged accounts.
- **Apply the principle of least privilege.** Grant users and services only the minimum permissions required to perform their tasks.
- **Regularly review access controls.** Audit roles and permissions on a recurring basis.

### Secrets Management

- **Never hardcode secrets.** Do not store API keys, passwords, tokens, or certificates in source code or configuration files committed to version control.
- **Use a dedicated secrets manager.** Store and retrieve secrets using tools such as HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, or Google Secret Manager.
- **Rotate secrets periodically.** Establish automated secret rotation policies to limit the impact of compromised credentials.
- **Use environment variables or secret injection.** Provide secrets to services at runtime through secure mechanisms rather than embedding them in artifacts.

### Network Security

- **Deploy within a private network.** Limit exposure of internal services to the public internet. Use VPCs, subnets, and security groups to isolate components.
- **Use a reverse proxy / API gateway.** Place services behind an API gateway (e.g., Nginx, Envoy, Kong) that handles TLS termination, rate limiting, and request validation.
- **Restrict inbound and outbound traffic.** Apply strict firewall rules and network policies to control traffic flow between services.
- **Enable network logging and monitoring.** Use tools such as intrusion detection systems (IDS) and network monitoring solutions to detect suspicious activity.

### Infrastructure & Deployment

- **Keep dependencies up to date.** Regularly update all libraries, frameworks, and base images to patch known vulnerabilities.
- **Scan for vulnerabilities.** Integrate container image scanning (e.g., Trivy, Snyk, Grype) into your CI/CD pipeline.
- **Run as non-root.** Ensure containers and processes run with the least privileged user possible.
- **Use immutable infrastructure.** Avoid in-place updates; prefer redeploying fresh instances when updating.

---

## Security Features

Netllm implements a range of built-in security features to protect against common threats:

| Feature               | Description                                                                                   |
| --------------------- | --------------------------------------------------------------------------------------------- |
| **JWT Authentication**      | Token-based authentication using JSON Web Tokens (JWT) with configurable expiration and rotation policies. |
| **Role-Based Access Control (RBAC)** | Fine-grained authorization system that restricts access to resources based on user roles and permissions. |
| **Rate Limiting**           | Configurable request rate limiting to prevent abuse, brute-force attacks, and denial of service. |
| **TLS Encryption**          | End-to-end encryption for data in transit using TLS 1.2+ with modern cipher suites. |
| **CORS Protection**         | Cross-Origin Resource Sharing (CORS) configuration to restrict access to trusted domains only. |
| **Input Validation**        | Comprehensive input validation and sanitization to prevent injection attacks (SQLi, XSS, command injection). |
| **Audit Logging**           | Detailed audit logs recording authentication events, access patterns, configuration changes, and administrative actions. |

> These features are designed to be configurable. Refer to the project documentation for guidance on enabling and tuning each security feature for your deployment.

---

## Disclosure Policy

We follow a **responsible disclosure** process to ensure that vulnerabilities are addressed safely and transparently.

### Our Commitment

1. **We will acknowledge** your report within 48 hours of receipt.
2. **We will investigate** the reported issue and keep you informed of our progress.
3. **We will not pursue legal action** against researchers who act in good faith and follow this disclosure policy.

### Disclosure Timeline

| Phase                          | Timeline                                    |
| ------------------------------ | ------------------------------------------- |
| **Report Received**            | Day 0                                       |
| **Acknowledgment**             | Within 48 hours                             |
| **Investigation & Fix**        | Based on severity (see response times above)|
| **Patch Release**              | After fix is validated and tested           |
| **Public Disclosure**          | 90 days after report, or after patch is released (whichever comes first) |

### Guidelines for Researchers

- **Do not access or modify** data that does not belong to you.
- **Do not degrade** system performance or availability.
- **Do not publicly disclose** the vulnerability until a patch has been released and users have had reasonable time to update.
- **Report only to the channels** listed in this policy.
- **Provide sufficient detail** to allow us to reproduce and fix the issue.

### Public Disclosure

Once a vulnerability has been patched and sufficient time has passed for users to upgrade:

1. We will publish a **security advisory** on GitHub.
2. We will credit the reporter (unless they prefer to remain anonymous).
3. We will update this document with any relevant changes to our security posture.

---

## Contact

For any security-related questions or concerns, please contact us at:

- **Email:** security@netllm.dev
- **GitHub Security Advisories:** [https://github.com/netllm/ai-provider/security/advisories](https://github.com/netllm/ai-provider/security/advisories)

---

*Last updated: 2025*
