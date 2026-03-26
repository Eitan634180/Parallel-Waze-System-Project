# 🚗 Parallel Waze System - Executive Project Presentation

Welcome to the **Parallel Waze System**, a high-performance, real-time navigation and traffic simulation engine built from the ground up. This document is designed as a comprehensive presentation guide, detailing every aspect of the project's architecture, features, and underlying algorithms.

---

## 🌟 1. Executive Summary
The Parallel Waze System is a custom-built, full-stack navigation platform that simulates a highly active road network. It goes beyond simple point-A to point-B routing by introducing **real-time traffic dynamics, live driver feedback loops, and concurrent background rerouting**. 

**Primary Objective:** To demonstrate advanced backend concurrency, efficient spatial graph algorithms, and real-time bidirectional WebSocket communication in a complex, live environment.

### 🎯 Key Features:
* **Real-time Map Rendering & Navigation:** A sleek Vanilla JS/HTML frontend displaying an interactive map and live driving dashboard.
* **Intelligent Route Planning:** Calculates the most efficient path utilizing highly optimized mapping data extracted directly from OpenStreetMap (OSM).
* **Live Traffic & Speed Feedback Loop:** As cars "drive" through the simulation, their speeds dynamically alter the "weight" (ETA) of the roads they use, propagating traffic jams across the network.
* **Dynamic Rerouting:** The engine constantly monitors active drivers; if traffic on their current route exceeds a threshold, a background process finds a faster alternative and updates them on-the-fly without interrupting their journey.
* **High-Concurrency Architecture:** Written entirely in Go, the server utilizes worker pools and non-blocking I/O to manage countless parallel simulation states without latency spikes.

---

## 🏗️ 2. System Architecture

The project is distinctly split into **Static Data Preparation** (Map Builder) and the **Live Real-time Environment** (Frontend UI + Go Server).

### High-Level Architecture
This logical overview demonstrates how the Vanilla JS Client interacts with the Go server, and how the Go server utilizes pre-processed OSM graph data.

```mermaid
graph TD
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

    subgraph "Map Data Pre-processing (CLI)"
        OSM[("OSM Data / PBF")]
        Builder["Map Builder"]
        GraphDB[("Compiled Overlay Graph (.bin)")]
    end

    UI --> WS_Service
    UI --> HTTP_Service
    UI --> MapRender
    WS_Service <-->|Real-time Updates| API
    HTTP_Service -->|Search & Config REST| API

    API --> SessionMgr
    API --> Sim
    SessionMgr <--> Routing
    SessionMgr <--> Traffic
    Sim --> Traffic
    Sim --> SessionMgr
    Routing --> Traffic

    OSM --> Builder
    Builder -->|Inertial Flow Partitioning| GraphDB
    GraphDB -->|Load on Startup| Routing

    class UI,WS_Service,HTTP_Service,MapRender client;
    class API,SessionMgr,Routing,Traffic,Sim server;
    class OSM,Builder preproc;
    class GraphDB db;
```

---

## ⚙️ 3. Backend Deep Dive: The Go Engine

The server (`/server`) is the powerhouse of the system. Designed in Go, it heavily leverages Goroutines and Channels to achieve massive parallelization.

### Concurrent Architecture Breakdown
This diagram perfectly illustrates our solution to handling thousands of concurrent WebSocket updates alongside heavy Dijkstra/A* graph calculations.

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

### Key Backend Components:
* **The Worker Pool & Channels:** Routing algorithms are computationally heavy. If the main server thread halted to calculate a new route every time a car hit traffic, the system would freeze. We solve this by dropping reroute requests into a **Channel**, which a dedicated pool of **Reroute Workers (Goroutines)** picks up, processes in the background, and seamlessly pushes back to the active session.
* **WebSocket I/O Pump:** A non-blocking architectural pattern designed to handle relentless, high-frequency GPS coordinate updates over WS, preventing system-wide latency spikes.
* **The Map Builder (Pre-processor):** Parses raw `.osm.pbf` files. We utilize **Inertial Flow Partitioning** to divide the massive map into manageable geographical cells, allowing the server to load a highly optimized one-level hierarchical overlay graph on startup instantly.

---

## 🚗 4. Core Flow: Real-Time Traffic & Dynamic Rerouting

The true "Waze" experience comes from our living traffic environment. Here is exactly what happens when a car starts driving:

```mermaid
sequenceDiagram
    participant Client as Dashboard (Client)
    participant API as Server WS API
    participant Session as Session Manager
    participant Traffic as Traffic Store
    participant Router as Routing Engine

    Client->>API: 1. Start Navigation
    API->>Session: 2. Create Tracking Session
    Session->>Router: 3. Calculate Initial Best Route
    Router-->>Session: Return Route Plan
    Session-->>API: Active Session Data
    API-->>Client: 4. Draw Route Polyline on UI

    loop Every Move Update
        Client->>API: 5. Send GPS/Location Update
        API->>Session: Update Vehicle Position
        Session->>Traffic: 6. Apply Speed Feedback Loop
        Traffic-->>Traffic: Adjust Edge Weight (Traffic Jam)
        
        Session->>Session: 7. Check Congestion/ETA threshold
        alt If threshold exceeded (Severe Congestion)
            Session->>Router: 8. Trigger Background Reroute Job (Worker)
            Router-->>Session: Return Optimized Alternative
            Session-->>API: Route Change Event
            API-->>Client: 9. Push New Route instruction
        else No severe congestion
            Session-->>API: Ping/Ack OK
        end
    end
```

### The Traffic Feedback Loop Explained:
1. **The Edge:** Every road segment in the graph is an "Edge" with a base speed limit and length.
2. **Speed Customization:** Through the dashboard, users can alter the speed of cars.
3. **The Feedback:** As a car moves slowly across an edge, the server tracks its velocity. It updates the central `Traffic Store`, explicitly raising the "Weight" (estimated time to cross) of that specific road.
4. **The Decay:** Traffic doesn't last forever. A `Background Decay Loop` constantly scrubs the `Traffic Store`, slowly lowering the artificial weights back down to their default, empty-road speeds over time.
5. **The Reroute Event:** As edge weights increase in the store, the `Session Manager` detects that an active driver's ETA has spiked. It asks the `Router` to check for alternatives. If a different path is faster, the system sends an update to the Client to change directions instantly.

---

## 💻 5. Frontend Deep Dive: The Dashboard

The UI (`/client`) is intentionally built with pure **Vanilla JavaScript, HTML, and CSS**, demonstrating fundamental DOM manipulation and direct WebSocket handling without the overhead of heavy frameworks. 

* **Collapsible UI:** A professional, responsive sidebar contains search capabilities, active car monitoring, and route information arrays.
* **Live Render Engine:** The map elements respond instantly to WebSocket events, dynamically drawing tracking lines, animating cars along polyline routes, and displaying active simulated driver statuses.

---

## 🚀 6. Getting Started / How to Run

Launching the entire system (Building the map, compiling the server, and serving the client) is fully automated.

### Prerequisites:
* **Go** installed and added to `PATH`.
* **Python 3** installed and added to `PATH` (used for the simple static HTTP server).
* **Map Data:** You must have the OpenStreetMap data packet (`israel-latest.osm.pbf`) located in `server\data\map\`.

### Execution:
1. Simply double-click the `run.cmd` file in the root directory, or execute it from the command line:
   ```cmd
   .\run.cmd
   ```
2. **What the script does:**
   * Validates Go and Python installations.
   * Checks if compiled `.bin` map files exist. If not, it natively runs the Map Builder to crunch the `.pbf` file.
   * Compiles the Go Server to an executable inside incredibly fast `.cache/bin/`.
   * Pops open two new command windows: One running the Go API (`:8080`) and one running the Python Static File Server (`:3000`).
   * Automatically opens your default web browser to `http://localhost:3000/navigation.html`.

Enjoy presenting the **Parallel Waze System**!
