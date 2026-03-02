# Swagger/OpenAPI & Auto-Generated Client SDKs

**Date**: 2026-03-02
**Purpose**: Define bridge API as OpenAPI spec and auto-generate client SDKs (industry standard)

---

## Overview

The bridge API is defined once in **OpenAPI/Swagger**, then **auto-generated client SDKs** are created for all platforms:

```
Bridge OpenAPI Spec (single source of truth)
         ↓
openapi-generator
         ↓
┌────────┬────────┬────────┬────────┐
│   JS   │ Swift  │ Kotlin │ Python │
│ Client │ Client │ Client │ Client │
└────────┴────────┴────────┴────────┘
         ↓
Developers use generated client (not manual HTTP calls)
```

**Benefits:**
- ✅ Single source of truth (swagger spec)
- ✅ Auto-generated clients (no manual coding)
- ✅ Consistent across platforms
- ✅ Type-safe (when generators support it)
- ✅ Easy to maintain (change spec → regenerate)
- ✅ Industry standard (AWS, Google, Azure, Stripe, GitHub)

---

## Bridge OpenAPI Specification

### File Structure

```
goop2-services/
├── https/
│   ├── openapi.yaml          # ← Bridge API specification
│   ├── config.go
│   ├── server.go
│   └── ...
├── cmd/
│   └── https-bridge-server/
│       ├── main.go
│       └── openapi.go        # ← Serve spec at /api/openapi.json
```

### OpenAPI Spec: https/openapi.yaml

```yaml
openapi: 3.0.3
info:
  title: Goop2 HTTPS Bridge API
  version: 1.0.0
  description: |
    Virtual peer gateway for mobile, web, and remote clients to participate
    in goop2's P2P network.

servers:
  - url: https://goop2.com
    description: Production
  - url: https://localhost:8804
    description: Development (local TLS)
  - url: http://localhost:8804
    description: Development (no TLS)

paths:
  /api/peers/{peerId}/connect:
    post:
      summary: Connect as a virtual peer
      description: |
        Create a virtual peer connection. Client provides peerId and metadata.
        Returns session details and active peer list.
      operationId: connectPeer
      tags:
        - Peers
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
            example: mobile-alice
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
            example: Bearer eyJhbGciOiJFZDI1NTE5IiwidHlwIjoiSldUIn0...
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [publicKey, metadata]
              properties:
                publicKey:
                  type: string
                  example: 12D3KooXxx...
                metadata:
                  type: object
                  properties:
                    name:
                      type: string
                      example: Alice's iPhone
                    avatar:
                      type: string
                      format: data-url
                    platform:
                      type: string
                      enum: [ios, android, web, desktop]
      responses:
        201:
          description: Virtual peer created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ConnectResponse'
        401:
          description: Unauthorized (invalid token)
        409:
          description: Peer already connected (different session)

  /api/peers/{peerId}/disconnect:
    post:
      summary: Disconnect virtual peer
      description: Gracefully disconnect and cleanup groups/subscriptions
      operationId: disconnectPeer
      tags:
        - Peers
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      responses:
        200:
          description: Disconnected

  /api/peers/{peerId}/ping:
    post:
      summary: Keep-alive ping
      description: Reset inactivity timeout, proves connection is alive
      operationId: pingPeer
      tags:
        - Peers
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      responses:
        204:
          description: Pong

  /api/peers/{peerId}/group/create:
    post:
      summary: Create a group
      operationId: createGroup
      tags:
        - Groups
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [groupId, name]
              properties:
                groupId:
                  type: string
                  example: group-project-backlog
                name:
                  type: string
                  example: Project Backlog
                role:
                  type: string
                  enum: [host, member]
                  default: host
      responses:
        201:
          description: Group created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Group'

  /api/peers/{peerId}/group/join:
    post:
      summary: Join a group
      operationId: joinGroup
      tags:
        - Groups
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [groupId]
              properties:
                groupId:
                  type: string
      responses:
        200:
          description: Joined group
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Group'

  /api/peers/{peerId}/group/leave:
    post:
      summary: Leave a group
      operationId: leaveGroup
      tags:
        - Groups
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [groupId]
              properties:
                groupId:
                  type: string
      responses:
        200:
          description: Left group

  /api/peers/{peerId}/message/send:
    post:
      summary: Send message to group
      operationId: sendMessage
      tags:
        - Messages
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [groupId, text]
              properties:
                groupId:
                  type: string
                text:
                  type: string
                mentions:
                  type: array
                  items:
                    type: string
      responses:
        201:
          description: Message sent
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Message'

  /api/peers/{peerId}/message/history:
    get:
      summary: Get group message history
      operationId: getMessageHistory
      tags:
        - Messages
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: groupId
          in: query
          required: true
          schema:
            type: string
        - name: limit
          in: query
          schema:
            type: integer
            default: 50
        - name: before
          in: query
          description: ISO8601 timestamp
          schema:
            type: string
            format: date-time
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      responses:
        200:
          description: Message history
          content:
            application/json:
              schema:
                type: object
                properties:
                  messages:
                    type: array
                    items:
                      $ref: '#/components/schemas/Message'

  /api/peers/{peerId}/realtime/subscribe:
    post:
      summary: Subscribe to realtime channel
      description: Open bidirectional realtime communication channel
      operationId: subscribeRealtime
      tags:
        - Realtime
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [channelId]
              properties:
                channelId:
                  type: string
                  example: rt-call-session-xyz
      responses:
        200:
          description: Subscribed to channel

  /api/peers/{peerId}/realtime/publish:
    post:
      summary: Publish to realtime channel
      operationId: publishRealtime
      tags:
        - Realtime
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [channelId, message]
              properties:
                channelId:
                  type: string
                message:
                  type: string
                  description: Message payload (arbitrary binary data as string)
                messageType:
                  type: string
                  enum: [offer, answer, ice, data]
      responses:
        200:
          description: Published

  /ws/peers/{peerId}:
    get:
      summary: WebSocket event stream
      description: |
        Open WebSocket connection for real-time event delivery.
        Client sends pings to keep-alive, server sends events.
      operationId: websocketEvents
      tags:
        - WebSocket
      parameters:
        - name: peerId
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: query
          required: true
          schema:
            type: string
            description: Bearer token as query param (WebSocket limitation)
      responses:
        101:
          description: Switching to WebSocket protocol

  /api/status:
    get:
      summary: Service status
      operationId: getStatus
      tags:
        - Service
      responses:
        200:
          description: Status OK
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Status'

  /healthz:
    get:
      summary: Health check
      operationId: healthCheck
      tags:
        - Service
      responses:
        200:
          description: Healthy

components:
  schemas:
    ConnectResponse:
      type: object
      properties:
        peerId:
          type: string
        status:
          type: string
          enum: [active, inactive]
        sessionId:
          type: string
        relayAddr:
          type: string
        peers:
          type: array
          items:
            $ref: '#/components/schemas/PeerInfo'

    PeerInfo:
      type: object
      properties:
        peerId:
          type: string
        lastSeen:
          type: string
          format: date-time
        metadata:
          type: object

    Group:
      type: object
      properties:
        groupId:
          type: string
        name:
          type: string
        members:
          type: array
          items:
            type: string
        createdAt:
          type: string
          format: date-time

    Message:
      type: object
      properties:
        messageId:
          type: string
        peerId:
          type: string
        text:
          type: string
        timestamp:
          type: string
          format: date-time

    Status:
      type: object
      properties:
        service:
          type: string
        version:
          type: string
        apiVersion:
          type: integer
        activePeers:
          type: integer
        uptime:
          type: string

  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
```

---

## Code Generation

### Step 1: Install openapi-generator

```bash
# macOS
brew install openapi-generator

# Linux
npm install -g @openapitools/openapi-generator-cli

# Or Docker
docker run -it openapitools/openapi-generator-cli
```

### Step 2: Generate Client SDKs

#### JavaScript/TypeScript

```bash
openapi-generator generate \
  -i https/openapi.yaml \
  -g javascript \
  -o generated/javascript-client \
  --package-name @goop/bridge-client \
  --package-version 1.0.0
```

**Output**: npm-ready package
```bash
cd generated/javascript-client
npm publish  # to npm registry
```

#### Swift (iOS)

```bash
openapi-generator generate \
  -i https/openapi.yaml \
  -g swift5 \
  -o generated/swift-client \
  --package-name GoopBridgeClient
```

**Output**: Swift Package
```swift
// Package.swift
.package(url: "https://github.com/goop/bridge-client-swift.git", from: "1.0.0")
```

#### Kotlin (Android)

```bash
openapi-generator generate \
  -i https/openapi.yaml \
  -g kotlin \
  -o generated/kotlin-client \
  --package-name com.goop.bridge
```

**Output**: Maven/Gradle artifact
```gradle
dependencies {
    implementation 'com.goop:bridge-client:1.0.0'
}
```

#### Python

```bash
openapi-generator generate \
  -i https/openapi.yaml \
  -g python \
  -o generated/python-client \
  --package-name goop-bridge-client
```

**Output**: pip-installable package
```bash
pip install goop-bridge-client
```

### Step 3: Publish SDKs

**JavaScript to npm:**
```bash
cd generated/javascript-client
npm publish
```

**Swift to GitHub (SPM):**
```bash
git push origin main
# GitHub release triggers SPM availability
```

**Kotlin to Maven Central:**
```bash
cd generated/kotlin-client
mvn clean deploy
```

---

## Generated Client Usage

### JavaScript/TypeScript

```javascript
import { GoopBridgeClient } from '@goop/bridge-client';

const client = new GoopBridgeClient({
  basePath: 'https://goop2.com'
});

// Connect as virtual peer
const response = await client.peersApi.connectPeer('mobile-alice', {
  publicKey: '12D3KooXxx...',
  metadata: {
    name: "Alice's iPhone",
    platform: 'ios'
  }
}, {
  headers: {
    'Authorization': 'Bearer ' + token
  }
});

console.log('Connected:', response.sessionId);

// Join group
await client.groupsApi.joinGroup('mobile-alice', {
  groupId: 'project-backlog'
}, {
  headers: { 'Authorization': 'Bearer ' + token }
});

// Send message
const msg = await client.messagesApi.sendMessage('mobile-alice', {
  groupId: 'project-backlog',
  text: 'Hello team!'
}, {
  headers: { 'Authorization': 'Bearer ' + token }
});

// Open WebSocket for events
const ws = new WebSocket(
  `wss://goop2.com/ws/peers/mobile-alice?authorization=Bearer+${token}`
);

ws.onmessage = (event) => {
  const eventData = JSON.parse(event.data);

  if (eventData.type === 'group_message') {
    console.log(`Message from ${eventData.peerId}: ${eventData.text}`);
  } else if (eventData.type === 'realtime_message') {
    console.log(`Realtime event: ${eventData.messageType}`);
  }
};
```

### Swift (iOS)

```swift
import GoopBridgeClient

let client = GoopBridgeClient(basePath: "https://goop2.com")

// Connect
let token = "Bearer eyJhbGciOiJFZDI1NTE5IiwidHlwIjoiSldUIn0..."

client.peersAPI.connectPeer(
  peerId: "ios-alice",
  connectRequest: ConnectRequest(
    publicKey: "12D3KooXxx...",
    metadata: ["name": "Alice's iPhone", "platform": "ios"]
  ),
  authorization: token
) { result in
  switch result {
  case .success(let response):
    print("Connected: \(response.sessionId)")

  case .failure(let error):
    print("Connection failed: \(error)")
  }
}

// Join group
client.groupsAPI.joinGroup(
  peerId: "ios-alice",
  joinGroupRequest: JoinGroupRequest(groupId: "project-backlog"),
  authorization: token
) { result in
  switch result {
  case .success:
    print("Joined group")
  case .failure(let error):
    print("Join failed: \(error)")
  }
}

// WebSocket for events
let ws = URLWebSocketTask(
  url: URL(string: "wss://goop2.com/ws/peers/ios-alice?authorization=\(token)")!
)

ws.onMessage { message in
  if case .string(let json) = message,
     let data = json.data(using: .utf8),
     let event = try? JSONDecoder().decode(Event.self, from: data) {

    switch event.type {
    case "group_message":
      print("Message: \(event.payload["text"] ?? "")")
    case "realtime_message":
      print("Realtime: \(event.payload["messageType"] ?? "")")
    default:
      break
    }
  }
}
```

### Kotlin (Android)

```kotlin
import com.goop.bridge.client.*

val client = GoopBridgeClient(basePath = "https://goop2.com")
val token = "Bearer eyJhbGciOiJFZDI1NTE5IiwidHlwIjoiSldUIn0..."

// Connect
lifecycleScope.launch {
  try {
    val response = client.peersApi.connectPeer(
      peerId = "android-alice",
      authorization = token,
      connectRequest = ConnectRequest(
        publicKey = "12D3KooXxx...",
        metadata = mapOf(
          "name" to "Alice's Pixel",
          "platform" to "android"
        )
      )
    )

    Log.d("Bridge", "Connected: ${response.sessionId}")

  } catch (e: Exception) {
    Log.e("Bridge", "Connection failed: $e")
  }
}

// Join group
lifecycleScope.launch {
  try {
    client.groupsApi.joinGroup(
      peerId = "android-alice",
      authorization = token,
      joinGroupRequest = JoinGroupRequest(groupId = "project-backlog")
    )
    Log.d("Bridge", "Joined group")
  } catch (e: Exception) {
    Log.e("Bridge", "Join failed: $e")
  }
}

// WebSocket for events (using OkHttp)
val webSocket = httpClient.newWebSocket(
  request = Request.Builder()
    .url("wss://goop2.com/ws/peers/android-alice?authorization=$token")
    .build(),
  listener = object : WebSocketListener() {
    override fun onMessage(webSocket: WebSocket, text: String) {
      val event = Json.decodeFromString<Event>(text)
      when (event.type) {
        "group_message" -> Log.d("Bridge", "Message: ${event.payload["text"]}")
        "realtime_message" -> Log.d("Bridge", "Realtime: ${event.payload["messageType"]}")
        else -> {}
      }
    }
  }
)
```

---

## Maintenance Workflow

### When API Changes

1. **Update openapi.yaml**
   ```yaml
   paths:
     /api/peers/{peerId}/new-endpoint:
       post:
         # New endpoint definition
   ```

2. **Regenerate SDKs**
   ```bash
   make generate-sdks
   ```

3. **Update SDK versions and publish**
   ```bash
   cd generated/javascript-client && npm publish
   cd generated/swift-client && git tag v1.1.0 && git push
   cd generated/kotlin-client && mvn deploy
   cd generated/python-client && twine upload
   ```

4. **Clients update dependencies**
   ```bash
   npm update @goop/bridge-client    # JavaScript
   pod update GoopBridgeClient        # Swift
   gradle dependencies --refresh       # Kotlin
   pip install --upgrade goop-bridge-client  # Python
   ```

### CI/CD Integration

**GitHub Actions workflow:**

```yaml
name: Generate and Publish SDKs

on:
  push:
    paths:
      - 'https/openapi.yaml'
    branches:
      - main

jobs:
  generate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Generate SDKs
        run: |
          make generate-sdks

      - name: Publish JavaScript SDK
        run: |
          cd generated/javascript-client
          npm publish
        env:
          NPM_TOKEN: ${{ secrets.NPM_TOKEN }}

      - name: Publish Swift SDK
        run: |
          cd generated/swift-client
          git config user.name "CI"
          git config user.email "ci@goop.com"
          git tag v${{ github.run_number }}
          git push origin v${{ github.run_number }}
```

---

## SDK Lifecycle

| Stage | Who | What |
|-------|-----|------|
| **1. Define** | Bridge team | Update `https/openapi.yaml` |
| **2. Generate** | CI/CD | Run `openapi-generator` |
| **3. Test** | Bridge team | Test generated code |
| **4. Publish** | CI/CD | Publish to npm, Maven, CocoaPods, PyPI |
| **5. Use** | Client teams | Update dependency, use generated API |
| **6. Repeat** | Cycle | When API changes, repeat 1-5 |

---

## Best Practices

### 1. Keep Spec as Source of Truth
```yaml
# Good: Spec is authoritative
openapi.yaml → (generate) → client SDKs

# Bad: Manual changes to generated code
client.js → (manual edits) → diverges from spec
```

### 2. Version SDKs with API
```bash
# SDK version matches API version
Bridge API 1.0.0 → @goop/bridge-client@1.0.0
Bridge API 1.1.0 → @goop/bridge-client@1.1.0
```

### 3. Use Semantic Versioning
```
1.2.3
│ │ └─ Patch (generated code cleanup)
│ └─── Minor (new endpoints added)
└───── Major (breaking changes)
```

### 4. Document Generated Code
```javascript
/**
 * Auto-generated from openapi.yaml
 * DO NOT edit this file manually
 *
 * To update:
 * 1. Update https/openapi.yaml
 * 2. Run: make generate-sdks
 * 3. Commit and push
 *
 * See: goop2-services/https/openapi.yaml
 */
```

---

## Comparison: Manual vs Generated

| Aspect | Manual Client | Generated Client |
|--------|---|---|
| **Creation** | Hand-write for each platform | One spec → N generators |
| **Consistency** | Error-prone (different implementations) | Guaranteed consistent |
| **Updates** | Edit each SDK separately | Regenerate all |
| **Documentation** | Can be outdated | Always in sync with API |
| **Type Safety** | Depends on language | Automatic from schema |
| **Time to New Platform** | Days/weeks | Minutes |
| **Industry Standard** | No | Yes (AWS, Google, Azure, Stripe) |

---

## Conclusion

By using **OpenAPI spec + auto-generation**:
- ✅ Single source of truth (openapi.yaml)
- ✅ Clients for JavaScript, Swift, Kotlin, Python, etc.
- ✅ Type-safe APIs
- ✅ Easy to maintain (change spec → regenerate)
- ✅ Industry standard approach
- ✅ Developers use generated SDK (not raw HTTP)

This is **the right way** to do it. 🎯
