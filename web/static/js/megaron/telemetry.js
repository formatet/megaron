// telemetry.js — umami event helper. Lowest layer (zero deps, like config.js)
// so every module may import it. The script tag (map.html / base.html) defines
// window.umami; an adblocker that blocks it leaves window.umami undefined, so
// this MUST swallow silently and never surface an error into game code.
// Policy (megaron_plan_umami.md): event-aggregates only, never player UUIDs.
export function track(name, props) {
  try { window.umami?.track(name, props); } catch (_) {}
}
