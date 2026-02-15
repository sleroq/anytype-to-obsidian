{
  lib,
  buildGoModule,
  version ? "dev",
}:

buildGoModule (finalAttrs: {
  pname = "anytype-to-obsidian";
  inherit version;

  src = lib.cleanSource ./.;

  vendorHash = "sha256-FabH6dC+vYeR2CTAqgk5oXdBJNK0YljTRoKPfmNIdXM=";

  ldflags = [
    "-s"
    "-w"
  ];

  doCheck = false;

  meta = {
    description = "Convert Anytype JSON export into Obsidian markdown";
    homepage = "https://github.com/sleroq/anytype-to-obsidian";
    mainProgram = "anytype-to-obsidian";
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
})
