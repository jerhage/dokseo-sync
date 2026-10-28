{
  lib,
  buildGoModule,
}:
let
  version = "0.1.3";
in
buildGoModule {
  pname = "dokseo-sync";
  inherit version;

  src = ../.;

  vendorHash = "sha256-sJn4yiUb227dGjpFF6dw/oXOLQ/2mdSikqymhkDchSM=";

  subPackages = [ "cmd/dokseo-sync" ];

  # Pure Go, so no C toolchain and a static binary.
  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X github.com/jerhage/dokseo-sync/internal/api.Version=${version}"
  ];

  meta = {
    description = "Sync server for Dokseo";
    homepage = "https://github.com/jerhage/dokseo-sync";
    license = lib.licenses.mit;
    mainProgram = "dokseo-sync";
  };
}
