# Quidditch Security Architecture

**Comprehensive Security Design for Enterprise Deployment**

**Version**: 1.0.0
**Date**: 2026-01-25

---

## Table of Contents

1. [Security Overview](#security-overview)
2. [Authentication](#authentication)
3. [Authorization](#authorization)
4. [Encryption](#encryption)
5. [Network Security](#network-security)
6. [Audit Logging](#audit-logging)
7. [Secrets Management](#secrets-management)
8. [Compliance](#compliance)
9. [Security Best Practices](#security-best-practices)

---

## Security Overview

### Security Layers

```
┌─────────────────────────────────────────────────────────┐
│                  Security Architecture                   │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  Layer 1: Network Security                               │
│  ├─ Kubernetes Network Policies                          │
│  ├─ Firewall Rules                                       │
│  └─ VPC/Private Subnets                                  │
│                                                           │
│  Layer 2: Transport Security                             │
│  ├─ TLS 1.3 (all communication)                          │
│  ├─ Certificate management (cert-manager)                │
│  └─ mTLS (inter-node)                                    │
│                                                           │
│  Layer 3: Authentication                                 │
│  ├─ JWT tokens                                           │
│  ├─ LDAP/Active Directory                                │
│  ├─ SAML 2.0 / OIDC                                      │
│  └─ API keys                                             │
│                                                           │
│  Layer 4: Authorization                                  │
│  ├─ Role-Based Access Control (RBAC)                     │
│  ├─ Attribute-Based Access Control (ABAC)                │
│  ├─ Field-Level Security                                 │
│  └─ Document-Level Security                              │
│                                                           │
│  Layer 5: Data Security                                  │
│  ├─ Encryption at rest (AES-256)                         │
│  ├─ Encryption in transit (TLS 1.3)                      │
│  ├─ Key rotation                                         │
│  └─ Secure deletion                                      │
│                                                           │
│  Layer 6: Audit & Monitoring                             │
│  ├─ Audit logs (immutable)                               │
│  ├─ Security events (SIEM integration)                   │
│  ├─ Anomaly detection                                    │
│  └─ Compliance reporting                                 │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

### Security Principles

1. **Zero Trust**: Never trust, always verify
2. **Defense in Depth**: Multiple layers of security
3. **Least Privilege**: Minimum necessary permissions
4. **Encryption Everywhere**: TLS for all communication
5. **Audit Everything**: Comprehensive logging
6. **Secure by Default**: Secure configuration out-of-box

---

## Authentication

### 1. JWT (JSON Web Tokens)

**Default Authentication Method**

**Configuration**:
```yaml
# quidditch-security.yaml
auth:
  jwt:
    enabled: true
    issuer: "https://auth.example.com"
    audience: "quidditch-api"
    jwks_url: "https://auth.example.com/.well-known/jwks.json"
    token_expiry: 3600  # 1 hour
    refresh_enabled: true
```

**Token Structure**:
```json
{
  "header": {
    "alg": "RS256",
    "typ": "JWT",
    "kid": "key-2026-01"
  },
  "payload": {
    "sub": "user123",
    "iss": "https://auth.example.com",
    "aud": "quidditch-api",
    "exp": 1706191200,
    "iat": 1706187600,
    "roles": ["user", "analyst"],
    "permissions": ["read:products", "write:logs"],
    "tenant_id": "acme-corp"
  },
  "signature": "..."
}
```

**Usage**:
```http
GET /my-index/_search HTTP/1.1
Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Implementation** (Go):
```go
type JWTAuthenticator struct {
    issuer   string
    audience string
    jwks     *keyfunc.JWKS
}

func (a *JWTAuthenticator) Authenticate(tokenString string) (*UserContext, error) {
    // Parse and validate token
    token, err := jwt.Parse(tokenString, a.jwks.Keyfunc)
    if err != nil {
        return nil, ErrInvalidToken
    }

    // Extract claims
    claims := token.Claims.(jwt.MapClaims)

    return &UserContext{
        UserID:      claims["sub"].(string),
        Roles:       toStringSlice(claims["roles"]),
        Permissions: toStringSlice(claims["permissions"]),
        TenantID:    claims["tenant_id"].(string),
    }, nil
}
```

---

### 2. LDAP / Active Directory

**Configuration**:
```yaml
auth:
  ldap:
    enabled: true
    url: "ldaps://ldap.example.com:636"
    bind_dn: "cn=admin,dc=example,dc=com"
    bind_password: "${LDAP_PASSWORD}"
    user_search_base: "ou=users,dc=example,dc=com"
    user_search_filter: "(uid={0})"
    group_search_base: "ou=groups,dc=example,dc=com"
    group_search_filter: "(member={0})"
```

**Authentication Flow**:
```
1. Client sends username + password
   ↓
2. Quidditch binds to LDAP with user credentials
   ↓
3. LDAP validates credentials
   ↓
4. Quidditch retrieves user groups
   ↓
5. Map groups to Quidditch roles
   ↓
6. Issue JWT token
```

---

### 3. SAML 2.0 / OIDC

**SAML Configuration**:
```yaml
auth:
  saml:
    enabled: true
    idp_metadata_url: "https://idp.example.com/metadata"
    sp_entity_id: "quidditch-prod"
    assertion_consumer_service_url: "https://quidditch.example.com/saml/acs"
    attribute_mapping:
      username: "urn:oid:0.9.2342.19200300.100.1.1"
      email: "urn:oid:0.9.2342.19200300.100.1.3"
      groups: "urn:oid:1.3.6.1.4.1.5923.1.5.1.1"
```

**OIDC Configuration**:
```yaml
auth:
  oidc:
    enabled: true
    issuer_url: "https://accounts.google.com"
    client_id: "quidditch-client-id"
    client_secret: "${OIDC_CLIENT_SECRET}"
    redirect_url: "https://quidditch.example.com/oidc/callback"
    scopes: ["openid", "profile", "email", "groups"]
```

---

### 4. API Keys

**For Programmatic Access**

**Configuration**:
```yaml
auth:
  api_keys:
    enabled: true
    key_prefix: "qd_"
    hash_algorithm: "sha256"
    storage: "etcd"  # or "database", "vault"
```

**API Key Structure**:
```
qd_live_abc123def456ghi789jkl012mno345pqr678
│   │    └─────────────────────────────────────┘
│   │              Key (random 32 bytes)
│   └─ Environment (live, test)
└─ Prefix
```

**Usage**:
```http
GET /my-index/_search HTTP/1.1
X-API-Key: qd_live_abc123def456ghi789jkl012mno345pqr678
```

**Creation**:
```http
POST /_security/api_key
Content-Type: application/json

{
  "name": "production-app-key",
  "role_descriptors": {
    "app-role": {
      "cluster": ["monitor"],
      "indices": [
        {
          "names": ["products-*"],
          "privileges": ["read", "view_index_metadata"]
        }
      ]
    }
  },
  "expiration": "30d"
}
```

**Response**:
```json
{
  "id": "abc123",
  "name": "production-app-key",
  "api_key": "qd_live_abc123def456ghi789jkl012mno345pqr678",
  "expiration": 1708783200000
}
```

---

## Authorization

### 1. Role-Based Access Control (RBAC)

**Roles**:
```yaml
roles:
  # Built-in roles
  - name: admin
    cluster_permissions:
      - all
    index_permissions:
      - indices: ["*"]
        privileges: [all]

  - name: analyst
    cluster_permissions:
      - cluster:monitor/*
    index_permissions:
      - indices: ["logs-*", "metrics-*"]
        privileges: [read, view_index_metadata]
      - indices: ["products"]
        privileges: [read]

  - name: developer
    cluster_permissions:
      - cluster:monitor/*
    index_permissions:
      - indices: ["dev-*"]
        privileges: [all]
      - indices: ["logs-*"]
        privileges: [read]

  # Custom role
  - name: ml_engineer
    cluster_permissions:
      - cluster:monitor/*
      - cluster:pipeline:create
      - cluster:pipeline:update
    index_permissions:
      - indices: ["ml-*", "features-*"]
        privileges: [read, write, create_index]
      - indices: ["models-*"]
        privileges: [all]
```

**Role Assignment**:
```http
PUT /_security/user/john
Content-Type: application/json

{
  "password": "securepassword",
  "roles": ["analyst", "ml_engineer"],
  "full_name": "John Doe",
  "email": "john@example.com"
}
```

---

### 2. Field-Level Security (FLS)

**Hide Sensitive Fields**

**Configuration**:
```yaml
roles:
  - name: customer_support
    index_permissions:
      - indices: ["customers"]
        privileges: [read]
        field_security:
          grant: ["customer_id", "name", "email", "orders"]
          except: ["ssn", "credit_card", "password_hash"]
```

**Implementation**:
```go
func (fls *FieldLevelSecurity) FilterDocument(doc map[string]interface{}, allowedFields []string) map[string]interface{} {
    filtered := make(map[string]interface{})

    for _, field := range allowedFields {
        if value, exists := doc[field]; exists {
            filtered[field] = value
        }
    }

    return filtered
}
```

---

### 3. Document-Level Security (DLS)

**Filter Documents by User**

**Configuration**:
```yaml
roles:
  - name: sales_rep
    index_permissions:
      - indices: ["orders"]
        privileges: [read]
        document_level_security:
          query: |
            {
              "term": {
                "sales_rep_id": "${user.id}"
              }
            }

  - name: tenant_user
    index_permissions:
      - indices: ["*"]
        privileges: [read, write]
        document_level_security:
          query: |
            {
              "term": {
                "tenant_id": "${user.tenant_id}"
              }
            }
```

**Implementation**:
```go
func (dls *DocumentLevelSecurity) ApplyFilter(req *SearchRequest, user *UserContext) {
    // Inject tenant filter
    tenantFilter := FilterNode{
        Term: &TermFilter{
            Field: "tenant_id",
            Value: user.TenantID,
        },
    }

    // Add to request filters
    req.Filters = append(req.Filters, tenantFilter)
}
```

---

## Encryption

### 1. Encryption at Rest

**Storage Encryption** (Kubernetes):
```yaml
apiVersion: v1
kind: StorageClass
metadata:
  name: encrypted-nvme
provisioner: ebs.csi.aws.com
parameters:
  type: io2
  encrypted: "true"
  kmsKeyId: "arn:aws:kms:us-east-1:123456789012:key/abc-123"
```

**Diagon Encryption**:
```cpp
// Encrypt segment files
class EncryptedIndexOutput : public IndexOutput {
private:
    std::unique_ptr<AES256Cipher> cipher_;
    std::vector<uint8_t> encryption_key_;

public:
    void writeBytes(const uint8_t* bytes, size_t len) override {
        std::vector<uint8_t> encrypted = cipher_->encrypt(bytes, len);
        underlying_output_->writeBytes(encrypted.data(), encrypted.size());
    }
};
```

**Configuration**:
```yaml
security:
  encryption:
    at_rest:
      enabled: true
      algorithm: "AES-256-GCM"
      key_provider: "kms"  # or "vault", "local"
      kms:
        region: "us-east-1"
        key_id: "arn:aws:kms:..."
```

---

### 2. Encryption in Transit

**TLS Configuration** (All Communication):

**Master Nodes**:
```yaml
master:
  tls:
    enabled: true
    certificate:
      secretName: master-tls-cert
    client_auth: "require"  # Mutual TLS
```

**Coordination Nodes**:
```yaml
coordination:
  tls:
    enabled: true
    certificate:
      secretName: coordination-tls-cert
    min_version: "TLS1.3"
    cipher_suites:
      - TLS_AES_256_GCM_SHA384
      - TLS_AES_128_GCM_SHA256
```

**Data Nodes**:
```yaml
data:
  tls:
    enabled: true
    certificate:
      secretName: data-tls-cert
    client_auth: "require"
```

**Certificate Generation** (cert-manager):
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: quidditch-tls
  namespace: quidditch
spec:
  secretName: quidditch-tls-cert
  issuer:
    name: letsencrypt-prod
    kind: ClusterIssuer
  commonName: quidditch.example.com
  dnsNames:
    - quidditch.example.com
    - "*.quidditch.example.com"
  privateKey:
    algorithm: RSA
    size: 4096
```

---

### 3. Key Management

**AWS KMS**:
```go
type KMSKeyProvider struct {
    client *kms.Client
    keyID  string
}

func (p *KMSKeyProvider) GenerateDataKey() (*DataKey, error) {
    result, err := p.client.GenerateDataKey(context.TODO(), &kms.GenerateDataKeyInput{
        KeyId:   aws.String(p.keyID),
        KeySpec: types.DataKeySpecAes256,
    })

    if err != nil {
        return nil, err
    }

    return &DataKey{
        Plaintext:  result.Plaintext,
        Ciphertext: result.CiphertextBlob,
    }, nil
}

func (p *KMSKeyProvider) DecryptDataKey(ciphertext []byte) ([]byte, error) {
    result, err := p.client.Decrypt(context.TODO(), &kms.DecryptInput{
        CiphertextBlob: ciphertext,
    })

    if err != nil {
        return nil, err
    }

    return result.Plaintext, nil
}
```

**Key Rotation**:
```yaml
security:
  key_rotation:
    enabled: true
    schedule: "0 0 1 * *"  # Monthly
    grace_period: "7d"      # Old keys valid for 7 days
```

---

## Network Security

### 1. Kubernetes Network Policies

**Isolate Data Nodes**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: data-node-policy
  namespace: quidditch
spec:
  podSelector:
    matchLabels:
      app: quidditch
      role: data
  policyTypes:
    - Ingress
    - Egress

  ingress:
    # Allow from coordination nodes
    - from:
      - podSelector:
          matchLabels:
            app: quidditch
            role: coordination
      ports:
        - protocol: TCP
          port: 9300

    # Allow from master nodes
    - from:
      - podSelector:
          matchLabels:
            app: quidditch
            role: master
      ports:
        - protocol: TCP
          port: 9300

    # Allow from Prometheus
    - from:
      - namespaceSelector:
          matchLabels:
            name: monitoring
      ports:
        - protocol: TCP
          port: 9600

  egress:
    # Allow to other data nodes
    - to:
      - podSelector:
          matchLabels:
            app: quidditch
            role: data
      ports:
        - protocol: TCP
          port: 9300

    # Allow to S3 (cold storage)
    - to:
      - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443

    # DNS
    - to:
      - namespaceSelector: {}
      ports:
        - protocol: UDP
          port: 53
```

---

### 2. Firewall Rules (Cloud Provider)

**AWS Security Groups**:
```yaml
# Coordination nodes (public)
ingress:
  - port: 9200
    protocol: TCP
    cidr: 0.0.0.0/0  # Public HTTP API
    description: "OpenSearch API"

# Data nodes (private)
ingress:
  - port: 9300
    protocol: TCP
    source_security_group: sg-coordination
    description: "Internal gRPC from coordination"

# Master nodes (private)
ingress:
  - port: 9301
    protocol: TCP
    source_security_group: sg-master
    description: "Raft consensus"
```

---

## Audit Logging

### 1. Audit Log Format

**Structure**:
```json
{
  "timestamp": "2026-01-25T10:30:45.123Z",
  "event_type": "search",
  "user": {
    "id": "user123",
    "username": "john.doe",
    "roles": ["analyst"],
    "ip_address": "192.168.1.100"
  },
  "request": {
    "id": "req-abc123",
    "method": "POST",
    "path": "/products/_search",
    "query_params": {},
    "body_hash": "sha256:..."
  },
  "response": {
    "status": 200,
    "duration_ms": 45,
    "hits": 152
  },
  "resource": {
    "type": "index",
    "name": "products"
  },
  "outcome": "success"
}
```

### 2. Audit Categories

**Authentication Events**:
- Login success/failure
- Token issued/refreshed/revoked
- Password changed
- MFA enabled/disabled

**Authorization Events**:
- Access granted/denied
- Permission check
- Role assigned/removed

**Data Access Events**:
- Search query
- Document read
- Document write/update/delete
- Bulk operation

**Administrative Events**:
- Index created/deleted
- Mapping updated
- Cluster settings changed
- Node added/removed

**Security Events**:
- Failed authentication (>3 attempts)
- Unauthorized access attempt
- Suspicious query patterns
- Certificate expiration warning

### 3. Audit Log Storage

**Configuration**:
```yaml
audit:
  enabled: true
  destinations:
    - type: file
      path: /var/log/quidditch/audit.log
      rotation:
        max_size: 100mb
        max_age: 90d
        max_files: 1000

    - type: syslog
      host: syslog.example.com
      port: 514
      protocol: tcp

    - type: elasticsearch
      hosts: ["audit-cluster:9200"]
      index: "quidditch-audit-%{+YYYY.MM.dd}"

  filters:
    - category: [authentication, authorization, admin]
      min_level: info
    - category: [data_access]
      min_level: debug
      sample_rate: 0.1  # 10% sampling for data access
```

---

## Secrets Management

### 1. Kubernetes Secrets

**Sealed Secrets**:
```yaml
apiVersion: bitnami.com/v1alpha1
kind: SealedSecret
metadata:
  name: quidditch-secrets
  namespace: quidditch
spec:
  encryptedData:
    db_password: AgBQZ3JhbnQg...
    jwt_secret: AgCkVhcm50Z...
    s3_access_key: AgDxNzQwMj...
```

**External Secrets Operator**:
```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: quidditch-secrets
  namespace: quidditch
spec:
  secretStoreRef:
    name: aws-secrets-manager
    kind: SecretStore
  target:
    name: quidditch-secrets
  data:
    - secretKey: db_password
      remoteRef:
        key: quidditch/prod/db_password
    - secretKey: jwt_secret
      remoteRef:
        key: quidditch/prod/jwt_secret
```

---

### 2. HashiCorp Vault Integration

**Configuration**:
```yaml
vault:
  enabled: true
  address: "https://vault.example.com:8200"
  auth_method: "kubernetes"
  role: "quidditch-prod"
  paths:
    database: "database/creds/quidditch"
    encryption: "transit/keys/quidditch-encryption"
    pki: "pki/issue/quidditch-cert"
```

**Usage** (Go):
```go
func (v *VaultClient) GetDatabaseCredentials() (*DBCredentials, error) {
    secret, err := v.client.Logical().Read("database/creds/quidditch")
    if err != nil {
        return nil, err
    }

    return &DBCredentials{
        Username: secret.Data["username"].(string),
        Password: secret.Data["password"].(string),
        LeaseID:  secret.LeaseID,
        TTL:      secret.LeaseDuration,
    }, nil
}
```

---

## Compliance

### 1. GDPR Compliance

**Right to Erasure**:
```http
# Delete all user data
DELETE /_security/user/john
DELETE /customers/_doc/user123
DELETE /logs/_delete_by_query
{
  "query": {
    "term": {"user_id": "user123"}
  }
}
```

**Data Export**:
```http
# Export user data
GET /customers/_search
{
  "query": {
    "term": {"user_id": "user123"}
  },
  "_source": true
}
```

**Audit Trail**:
- All data access logged
- Retention period: 90 days minimum
- Immutable audit logs

---

### 2. HIPAA Compliance

**Required Controls**:
- ✅ Encryption at rest (AES-256)
- ✅ Encryption in transit (TLS 1.3)
- ✅ Access controls (RBAC + audit)
- ✅ Audit logging (comprehensive)
- ✅ Data backup (encrypted snapshots)
- ✅ Disaster recovery (tested procedures)

**PHI Handling**:
```yaml
indices:
  - name: patient-records
    security:
      encryption: required
      field_level_security:
        sensitive_fields: ["ssn", "diagnosis", "medical_history"]
      document_level_security:
        provider_filter: true
      audit:
        log_all_access: true
```

---

### 3. SOC 2 Compliance

**Type II Controls**:
- **CC6.1**: Logical access controls
- **CC6.2**: Prior to issuing credentials
- **CC6.3**: Removes access when no longer required
- **CC6.6**: Protects information at rest
- **CC6.7**: Protects information in transmission
- **CC7.2**: Monitors system activity

**Evidence Collection**:
```yaml
compliance:
  soc2:
    enabled: true
    evidence_collection:
      - access_reviews: monthly
      - vulnerability_scans: weekly
      - penetration_tests: quarterly
      - audit_log_reviews: daily
```

---

## Security Best Practices

### 1. Secure Deployment Checklist

- [ ] **Authentication**
  - [ ] Enable JWT authentication
  - [ ] Configure LDAP/SAML/OIDC
  - [ ] Enforce strong password policy
  - [ ] Enable MFA for admin accounts

- [ ] **Authorization**
  - [ ] Define custom roles
  - [ ] Implement least privilege
  - [ ] Enable field-level security
  - [ ] Enable document-level security

- [ ] **Encryption**
  - [ ] Enable TLS 1.3 (all communication)
  - [ ] Enable encryption at rest
  - [ ] Rotate encryption keys quarterly
  - [ ] Use strong cipher suites

- [ ] **Network**
  - [ ] Configure network policies
  - [ ] Isolate data nodes
  - [ ] Use private subnets
  - [ ] Enable firewall rules

- [ ] **Audit**
  - [ ] Enable comprehensive audit logging
  - [ ] Configure SIEM integration
  - [ ] Set up alerts for security events
  - [ ] Test audit log retention

- [ ] **Secrets**
  - [ ] Use external secrets manager (Vault, AWS Secrets Manager)
  - [ ] Never commit secrets to git
  - [ ] Rotate secrets regularly
  - [ ] Use sealed secrets in Kubernetes

- [ ] **Monitoring**
  - [ ] Set up security dashboards
  - [ ] Configure anomaly detection
  - [ ] Alert on failed authentication
  - [ ] Monitor certificate expiration

---

### 2. Incident Response

**Security Incident Response Plan**:

1. **Detection** (< 5 minutes)
   - Automated alerts trigger
   - Security team notified

2. **Containment** (< 30 minutes)
   - Isolate affected nodes
   - Revoke compromised credentials
   - Block suspicious IPs

3. **Investigation** (< 2 hours)
   - Analyze audit logs
   - Identify attack vector
   - Assess damage

4. **Remediation** (< 24 hours)
   - Patch vulnerabilities
   - Rotate all credentials
   - Update firewall rules

5. **Recovery** (< 48 hours)
   - Restore from backups if needed
   - Verify system integrity
   - Resume normal operations

6. **Post-Incident** (< 1 week)
   - Root cause analysis
   - Update security policies
   - Conduct security training

---

### 3. Security Hardening

**System Hardening**:
```yaml
# Disable unnecessary features
features:
  swagger_ui: false
  debug_endpoints: false
  profiling: false

# Rate limiting
rate_limiting:
  enabled: true
  requests_per_second: 100
  burst: 200

# Request size limits
limits:
  max_request_size: 10mb
  max_query_complexity: 1000
  max_aggregation_buckets: 10000

# Timeouts
timeouts:
  query: 30s
  bulk: 60s
  scroll: 300s
```

---

**Version**: 1.0.0
**Last Updated**: 2026-01-25
**Classification**: Security Architecture
