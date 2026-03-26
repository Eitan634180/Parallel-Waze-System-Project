# Parallel-Waze-System-Project

This repository contains the **Parallel Waze System**. Below is a comprehensive visual and structural breakdown of the project architecture, including the components, concurrent architecture, and sequence flows of the application.

## 1. High-Level System Architecture

This diagram shows the top-level communication between the various subsystems that make up the project: the Map Builder (pre-processing), the Navigation Server (Go backend), and the Dashboard Client (Web frontend).

```mermaid
graph TD
    %% Define styles
    classDef client fill:#d4edda,stroke:#28a745,stroke-width:2px;
    classDef server fill:#cce5ff,stroke:#007bff,stroke-width:2px;
    classDef preproc fill:#fff3cd,stroke:#ffc107,stroke-width:2px;
    classDef db fill:#f8d7da,stroke:#dc3545,stroke-width:2px;

    subgraph "Frontend Client (Vanilla JS/HTML/CSS)"
        UI["Dashboard UI"]
        WS_Service["WebSocket Service"]
        HTTP_Service["HTTP Rest Client"]
        MapRender["Map Rendering Engine"]
    end

    subgraph "Navigation Server (Go)"
        API["API Layer: HTTP & WS"]
        SessionMgr["Session Manager"]
        Routing["Routing Engine"]
        Traffic["Traffic Store"]
        Sim["Simulation Manager"]
    end

    subgraph "Map Data Pre-processing"
        OSM[("OSM Data / PBF")]
        Builder["Map Builder"]
        GraphDB[("Compiled Overlay Graph")]
    end

    %% Client communicating to Server
    UI --> WS_Service
    UI --> HTTP_Service
    UI --> MapRender
    WS_Service <-->|Real-time Updates| API
    HTTP_Service -->|Search & Config REST| API

    %% Server Internals
    API --> SessionMgr
    API --> Sim
    SessionMgr <--> Routing
    SessionMgr <--> Traffic
    Sim --> Traffic
    Sim --> SessionMgr
    Routing --> Traffic

    %% Pre-processing
    OSM --> Builder
    Builder -->|Inertial Flow Partitioning| GraphDB
    GraphDB -->|Load on Startup| Routing

    %% Apply Classes
    class UI,WS_Service,HTTP_Service,MapRender client;
    class API,SessionMgr,Routing,Traffic,Sim server;
    class OSM,Builder preproc;
    class GraphDB db;
```

---

## 2. Server Concurrency & Internal Components

The server is designed for high concurrency and low-latency route optimization. This diagram illustrates the Go-specific concurrency patterns utilized, specifically the WebSocket write pools and background worker pools.

```mermaid
graph TB
    classDef interface fill:#e2e3e5,stroke:#383d41;
    classDef worker fill:#d1ecf1,stroke:#17a2b8;
    classDef store fill:#f8d7da,stroke:#dc3545;

    Client(("Web Client")) <-->|WS connection| WSPump["WebSocket I/O Pump"]
    Client -->|HTTP GET/POST| HTTPHandlers["HTTP Handlers"]

    subgraph "API Layer"
        WSPump
        HTTPHandlers
    end

    subgraph "Session Manager"
        SM_Core["Session State Maps"]
        RerouteWorkers["Reroute Worker Pool (Channels)"]
        EventLoop["Main Event Loop"]
    end

    subgraph "Traffic Engine"
        TStore[("Traffic Store")]
        DecayLoop["Background Decay Loop"]
        Customizer["Speed Customization"]
    end

    subgraph "Routing Graph"
        Router["A* / Dijkstra Search"]
        GraphMem[("In-Memory Overlay Graph")]
    end

    WSPump -->|Update / Move| EventLoop
    EventLoop --> SM_Core
    SM_Core -->|Queue Job| RerouteWorkers
    
    RerouteWorkers -->|Calculate Alt Routes| Router
    Router -->|Read Weights| TStore
    Router -->|Read Topology| GraphMem

    HTTPHandlers --> Router

    TStore <--> Customizer
    TStore --- DecayLoop

    class WSPump,HTTPHandlers interface;
    class RerouteWorkers worker;
    class TStore,GraphMem store;
```

### Component Breakdown
* **WebSocket I/O Pump:** A non-blocking architectural pattern designed to handle network I/O smoothly, preventing system-wide latency spikes.
* **Worker Pool:** A pool of Goroutines that pull routing tasks from channels, processing background route optimizations and ETA updates for active drivers concurrently.
* **Traffic Store & Decay Loop:** Holds real-time Edge speed limits/delays. A background loop automatically decays these values over time back to default mapping speeds.
* **In-Memory Overlay Graph:** The primary navigation graph loaded at startup, highly optimized using one-level hierarchical overlay mapping and Inertial Flow partitioning.

---

## 3. Core Flow: Real-Time Driving and Rerouting

Visualizing the system flow when a client connects, drives along a route, and potentially requires dynamic rerouting due to changing traffic conditions.

```mermaid
sequenceDiagram
    participant Client as Dashboard (Client)
    participant API as Server WS API
    participant Session as Session Manager
    participant Traffic as Traffic Store
    participant Router as Routing Engine

    Client->>API: 1. Start Navigation (Origin, Dest)
    API->>Session: 2. Create Session
    Session->>Router: 3. Calculate Initial Route
    Router-->>Session: Return Route Plan
    Session-->>API: Active Session Data
    API-->>Client: 4. Route Details & Polyline

    loop Every Move Update
        Client->>API: 5. Send GPS/Location Update
        API->>Session: Update Vehicle Position
        Session->>Traffic: 6. Apply Speed Feedback Loop
        Traffic-->>Traffic: Adjust Edge Weight (ETA)
        
        Session->>Session: 7. Check Congestion/ETA threshold
        alt If threshold exceeded (Congestion)
            Session->>Router: 8. Trigger Background Rerouting Job (Worker)
            Router-->>Session: Return Optimized Alternative
            Session-->>API: Route Change Event
            API-->>Client: 9. Push New Route instruction
        else No severe congestion
            Session-->>API: Ping/Ack OK
        end
    end
```

### Sequence Explanation:
1. **Start Navigation:** The user selects a route on the Dashboard.
2. **Initial Plan:** The `Session Manager` tasks the `Router` to find the initial best path and creates a tracking session.
3. **Move Update Iteration:** The client sends simulated GPS updates over WebSocket as they 'drive'.
4. **Traffic Influence (Feedback Loop):** The server analyzes the user's progressing speed and updates the central `Traffic Store`, potentially altering ETA for other drivers on the same road segment (Edge).
5. **Background Rerouting:** The `Session Manager` constantly evaluates the route. If traffic spikes, a non-blocking `Goroutine` handles calculating a new, faster path dynamically without interrupting the current drive flow.

---

## Summary
The **Parallel Waze System** is split distinctly into static data generation and live real-time interaction. It leverages **Go's strong concurrency features** (Channels, Goroutines) to manage numerous cars, calculate continuous spatial graphs, and distribute data updates via WebSockets down to a responsive Vanilla JS client interface.
