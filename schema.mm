graph TD
    subgraph Cloud API
        SERVER[Server<br/>Port: 8080<br/>Runs: liveops]
        DB[Database] --> SERVER
        DISK1[Storage 1] --> SERVER
        DISK2[Storage 2] --> DB
        
        SERVER -->|HTTP: GET /events, /events/id| HTTP_EP[HTTP Endpoint<br/>Rate Limited]
        SERVER -->|gRPC: Create, Update,<br/>Delete, List| GRPC_EP[gRPC Endpoint<br/>Admin Access]
    end

    subgraph Docker Compose
        STRESS[Stress Client<br/>stressclient] -->|HTTP| HTTP_EP
        STRESS -->|gRPC| GRPC_EP
        CLI[CLI Tool<br/>liveops-cli] -->|HTTP| HTTP_EP
        CLI -->|gRPC| GRPC_EP
        
        SERVER -.-|go-nacon| STRESS
        SERVER -.-|go-nacon| CLI
    end

    subgraph Host
        DC[Docker Compose<br/>docker-compose.yml] -->|Deploys| SERVER
        DC -->|Deploys| STRESS
        DC -->|Deploys| CLI
        USER[User] -->|Runs| DC
        USER -->|Interacts| CLI
        USER -->|Monitors| STRESS
    end

    subgraph Repo: /Code/liveops
        ROOT[Root]
        ROOT --> CMD[cmd/]
        ROOT --> API[api/]
        CMD --> S1[server/main.go<br/>Server]
        CMD --> S2[stressclient/main.go<br/>Stress]
        CMD --> S3[cli/main.go<br/>CLI]
        API --> P[api.proto<br/>gRPC]
        ROOT --> D[Dockerfile<br/>Build]
        ROOT --> C[docker-compose.yml<br/>Config]
        ROOT --> A[architecture-beta.mmd<br/>Diagram]
    end

    %% Styling
    classDef service fill:#f9f,stroke:#333,stroke-width:2px;
    classDef endpoint fill:#bbf,stroke:#333,stroke-width:2px;
    classDef host fill:#dfd,stroke:#333,stroke-width:2px;
    classDef file fill:#ff9,stroke:#333,stroke-width:2px;
    class SERVER,STRESS,CLI,DB,DISK1,DISK2 service;
    class HTTP_EP,GRPC_EP endpoint;
    class DC,USER host;
    class ROOT,CMD,API,S1,S2,S3,P,D,C,A file;