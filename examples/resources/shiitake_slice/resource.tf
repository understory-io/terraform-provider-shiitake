// The canopy-models slices, as desired state.
//
// One entry per slice. Removing an entry prunes the slice: scheduling stops and
// the task definition is marked for teardown. That is the property worth having
// — before this, a slice existed because somebody remembered to run
// `shiitake schedule register`, and external/exchange-rates proved how that
// fails (images, contracts and grants all shipped; the slice never ran).

locals {
  # Pin the IMMUTABLE tag, not `:main`.
  #
  # This is load-bearing. Registration is an upsert keyed on the spec, so a spec
  # that still says `:main` is unchanged even when the tag now points at a new
  # build — the server sees no diff, provisions nothing and launches nothing.
  # Terraform would likewise report "No changes" forever while the slice ran
  # last month's image. `build-slices.yml` pushes `:main`, `:<timestamp>` and
  # `:<short-sha>`; the sha is the one that makes a deploy a deploy.
  canopy_models_sha = "3e24256"

  image_base = "ghcr.io/understory-io/canopy-models"

  slices = {
    google-ads-stream = { domain = "marketing", path = "marketing/google-ads/streaming" }
    google-ads-batch  = { domain = "marketing", path = "marketing/google-ads/batch" }

    # DATA-590: registered late. Their images had existed since DATA-587 and
    # nothing ran them, so `dataset://external/fx-rates` was a live, published,
    # subscribable contract with no table behind it.
    exchange-rates-stream = { domain = "external", path = "external/exchange-rates/streaming" }
    exchange-rates-batch  = { domain = "external", path = "external/exchange-rates/batch" }

    bookings-stream = { domain = "payments", path = "payments/bookings/streaming" }
    bookings-batch  = { domain = "payments", path = "payments/bookings/batch", schedule = "*/5 * * * *" }

    # Consumes every dataset port exposed to Metabase, so it runs last.
    bi = { domain = "analytics", path = "analytics/bi/batch" }
  }
}

resource "shiitake_slice" "canopy_models" {
  for_each = local.slices

  name    = each.key
  project = "canopy-models"
  domain  = each.value.domain
  image   = "${local.image_base}/${each.value.path}:${local.canopy_models_sha}"

  # Empty means one-shot: it runs when registered rather than on a timer.
  schedule = try(each.value.schedule, "")

  # 1024/2048 is what the slices that have been running in anger use. The two
  # exchange-rates slices were first registered by hand on the CLI's 256/512
  # defaults, which is thin for a first run that unnests ~283k rows of history.
  cpu  = try(each.value.cpu, "1024")
  mem  = try(each.value.mem, "2048")
  arch = "x86_64"
}
