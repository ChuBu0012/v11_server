# Location Place Picker UX Design

## Intent

Make adding a geofence understandable and easy on desktop and mobile. The map stays the source of truth for the selected location, while the form guides the user through a clear sequence.

## Goals

- Select a place by moving the map center and seeing a live radius preview.
- Offer shortcuts to the latest Chubu and Furturia locations.
- Guide the user through location, name/radius, and notification choices in order.
- Make radius adjustable with a slider and an exact numeric value.
- Make saved places scannable and allow a place card to focus the map.
- Keep controls comfortable on touch screens and accessible by keyboard.

## Non-goals

- No external geocoding or address search; place names and coordinates stay in Pocket Sentinel.
- No API or database changes.
- No editing existing geofence coordinates/radius in this pass; existing delete behavior remains available with confirmation.

## Approaches

1. **Map-first guided panel (chosen):** a side panel on desktop and full-width bottom sheet on mobile; the map center is the selected point, device shortcuts center it on the latest positions, and a live circle previews the radius. A compact three-step flow handles place, radius, and notification choices.
2. **External address search:** supports lookup by business/address but sends searches to an external geocoder and adds provider/privacy constraints. Not chosen because the user selected a local map-and-device workflow.

## Behavior and layout

- Opening “Add place” enters selection mode and presents short instructions.
- The user can move the map under a fixed center reticle or choose Chubu/Furturia as the center. Coordinates remain visible as a confirmation detail.
- Step 2 asks for a required place name and radius. A range slider updates the preview circle immediately; a labeled numeric field permits precise values. Common values are available as shortcuts.
- Step 3 contains independent arrival/departure notification toggles and the PIN when needed, followed by one primary save action and a clear success/error state.
- Desktop uses a right-side panel that leaves the map visible. Mobile uses a bottom sheet with safe-area padding and a clear close/cancel action.
- Saved places display as ordered cards with name, radius, enabled alert types, and coordinates. Selecting a card focuses the map on its circle. Deletion requires confirmation.
- Leaflet objects remain in the outer `liveMapState` object, outside Alpine reactive state. Frontend changes bump the service worker cache version.

## Constraints and verification

- Reuse the existing OwnTracks latest-location API and PIN-protected geofence creation endpoint.
- Preserve current geofence semantics and dashboard styling.
- Verify JavaScript syntax, dashboard and location tests, service-worker version/cache assets, keyboard/ARIA behavior, and responsive layout at small mobile and desktop widths.
