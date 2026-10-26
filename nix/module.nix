{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.dokseo-sync;
  stateDir = "/var/lib/dokseo-sync";
in
{
  options.services.dokseo-sync = {
    enable = lib.mkEnableOption "the Dokseo sync server";

    package = lib.mkPackageOption pkgs "dokseo-sync" { };

    listen = lib.mkOption {
      type = lib.types.str;
      default = "127.0.0.1:8080";
      description = ''
        Address to listen on. Loopback by default, on the assumption that a
        reverse proxy terminates TLS; the server speaks plain HTTP.
      '';
    };

    dataDir = lib.mkOption {
      type = lib.types.str;
      default = stateDir;
      description = ''
        Directory holding `dokseo-sync.db`. The default is created and owned by
        systemd; any other path has to exist and be writable by the service.
      '';
    };

    username = lib.mkOption {
      type = lib.types.str;
      example = "me";
      description = "The server's single account.";
    };

    passwordFile = lib.mkOption {
      type = lib.types.path;
      example = "/run/secrets/dokseo-sync-password";
      description = ''
        File holding that account's password, read by root and passed in as a
        systemd credential, so the service user never needs access to it. One
        trailing newline is ignored.
      '';
    };

    corsOrigins = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      example = [ "https://reader.example.com" ];
      description = ''
        Browser origins allowed to call the API, as `scheme://host[:port]` with
        no path or trailing slash.

        The server sends its own CORS headers, so a reverse proxy in front of
        it must not add any: two Access-Control-Allow-Origin headers make the
        browser reject the response.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.dokseo-sync = {
      description = "Dokseo sync server";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

      serviceConfig = {
        ExecStart = lib.escapeShellArgs (
          [
            (lib.getExe cfg.package)
            "--listen"
            cfg.listen
            "--data"
            cfg.dataDir
            "--username"
            cfg.username
            "--password-file"
            "%d/password"
          ]
          ++ lib.concatMap (o: [
            "--cors-origin"
            o
          ]) cfg.corsOrigins
        );
        LoadCredential = "password:${cfg.passwordFile}";
        Restart = "on-failure";

        DynamicUser = true;
        PrivateUsers = true;

        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectKernelLogs = true;
        ProtectControlGroups = true;
        ProtectClock = true;
        ProtectHostname = true;
        ProtectProc = "invisible";
        ProcSubset = "pid";
        NoNewPrivileges = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        RestrictNamespaces = true;
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
        ];
        SystemCallArchitectures = "native";
        SystemCallFilter = [
          "@system-service"
          "~@privileged"
          "~@resources"
        ];
        CapabilityBoundingSet = [ "" ];
        UMask = "0077";
      }
      // (
        if cfg.dataDir == stateDir then
          { StateDirectory = "dokseo-sync"; }
        else
          { ReadWritePaths = [ cfg.dataDir ]; }
      );
    };
  };
}
