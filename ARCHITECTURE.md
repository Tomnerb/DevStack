# Milestone 16.1.3 — Linux Docker Desktop Lifecycle

Installed state and running state are now independent:

```text
systemd user unit loaded?
       │
       └── EngineInstalled

Docker API socket responds?
       │
       └── Running
```

Start is non-blocking:

```text
Start Engine
    │
    ├─ systemctl --user start --no-block docker-desktop
    │
    └─ poll configured Desktop socket (~20 sec max)
```

The selected Docker engine identity from Milestone 16.1.2 remains preserved.
