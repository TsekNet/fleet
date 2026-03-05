# Custom notifications

_Available in Fleet Premium_

In Fleet, you can send custom notifications to end users on macOS, Windows, and Linux hosts. Notifications are rich dialogs with configurable headings, messages, buttons, deferrals, image carousels, and filesystem watch paths.

Fleet uses [hermes](https://github.com/TsekNet/hermes), a TUF-managed notification binary that Orbit downloads and launches in the user's session via `execuser`. This is the same pattern used by [Nudge](https://github.com/macadmins/nudge) for macOS update enforcement and [swiftDialog](https://github.com/swiftDialog/swiftDialog) for setup experience status.

## How it works

1. A Fleet admin defines notification templates in GitOps YAML (under `controls.notifications`) or via the REST API.
2. Notification templates are linked to policies. When a policy fails for a host, the corresponding notification becomes pending.
3. When Orbit polls the server for its config (every 30 seconds), the server includes any pending notification IDs in the response.
4. Orbit downloads the `hermes` binary from TUF (if not already cached), writes the notification config to a local JSON file, and launches the binary in the user's session with `--local` mode.
5. The end user sees the notification dialog and can interact with it (click a button, defer, or let it time out).
6. The user's response is reported back to the Fleet server for tracking.

## Notification templates

Notification templates are defined in your GitOps YAML under `controls.notifications`. Each template specifies the content and behavior of the notification dialog.

### Minimal example

```yaml
controls:
  notifications:
    - path: ./notifications/restart-required.yml
```

Where `./notifications/restart-required.yml` contains:

```yaml
id: restart-required
heading: Restart Required
message: >
  Your computer needs to restart to apply security updates.
  Please save your work and restart at your earliest convenience.
buttons:
  - label: Restart Now
    value: "url:ms-settings:windowsupdate"
    style: primary
  - label: Defer
    style: secondary
    dropdown:
      - label: 1 Hour
        value: defer_1h
      - label: 4 Hours
        value: defer_4h
      - label: Tomorrow
        value: defer_1d
timeout: 300
timeout_value: defer_1h
defer_deadline: 7d
max_defers: 5
accent_color: "#76B900"
```

### Full template reference

| Field | Type | Required | Default | Description |
| ----- | ---- | -------- | ------- | ----------- |
| `id` | string | Yes | - | Unique identifier for delivery tracking. Alphanumeric, hyphens, underscores, and dots only. |
| `heading` | string | Yes | - | Bold heading at the top of the notification. |
| `message` | string | No | `""` | Body text below the heading. |
| `buttons` | list | No | Single "OK" button | Clickable actions. See [Buttons](#buttons). |
| `timeout` | integer | No | `300` | Seconds before auto-actioning with `timeout_value`. |
| `timeout_value` | string | No | `""` | Value reported when the timeout fires. |
| `esc_value` | string | No | Same as `timeout_value` | Value reported when the user presses ESC or closes the window. |
| `title` | string | No | `"IT Department"` | Small label at the top of the notification window. |
| `accent_color` | string | No | `"#D4A843"` | Hex color for the accent bar and primary buttons (e.g. `#76B900`). |
| `help_url` | string | No | `""` | "Need help?" link in the header. Must be an `http` or `https` URL. |
| `defer_deadline` | string | No | `""` | Maximum duration for deferrals (e.g. `24h`, `7d`). |
| `max_defers` | integer | No | `0` | Maximum number of deferrals. `0` means unlimited (until deadline). |
| `images` | list | No | `[]` | HTTPS image URLs or `data:image/` URIs for a carousel (max 20). SVG data URIs are not allowed. |
| `watch_paths` | list | No | `[]` | Filesystem paths to monitor for changes (max 10). Path traversal (`..`) is not allowed. |
| `dnd` | string | No | `"respect"` | Behavior when Do Not Disturb is active: `respect` (wait), `ignore` (show anyway), or `skip` (silently complete with `dnd_active`). |

### Buttons

Each button has the following fields:

| Field | Type | Required | Default | Description |
| ----- | ---- | -------- | ------- | ----------- |
| `label` | string | Yes | - | Button text. |
| `value` | string | No | Lowercase label | Value reported when clicked. Prefix with `url:` to open a URL. |
| `style` | string | No | `"secondary"` | `primary`, `secondary`, or `danger`. |
| `dropdown` | list | No | `[]` | Dropdown options (replaces the button click with a dropdown menu). |

Each dropdown option has `label` (string) and `value` (string).

### Action prefixes

Button values can use the following prefixes:

| Prefix | Behavior |
| ------ | -------- |
| `url:` | Opens the URL in the default browser. The notification stays open so the user can click other buttons. |
| (none) | Reports the raw value back to the Fleet server. |

## Linking notifications to policies

Notifications are activated when a policy fails. In your GitOps YAML, reference the notification template ID in the policy definition:

```yaml
policies:
  - name: Restart pending
    query: SELECT 1 FROM uptime WHERE total_seconds > 604800;
    notification_id: restart-required
```

When this policy fails for a host, the `restart-required` notification becomes pending and is delivered on the next Orbit config poll.

## Configuring via REST API

### Create or update a notification template

`POST /api/v1/fleet/notifications`

```json
{
  "id": "restart-required",
  "heading": "Restart Required",
  "message": "Your computer needs to restart to apply security updates.",
  "buttons": [
    {"label": "Restart Now", "value": "restart", "style": "primary"},
    {"label": "Defer 1h", "value": "defer_1h", "style": "secondary"}
  ],
  "timeout": 300,
  "timeout_value": "defer_1h"
}
```

### Get notification config for a host (Orbit-authenticated)

`POST /api/fleet/orbit/notification_config`

```json
{
  "orbit_node_key": "...",
  "notification_id": "restart-required"
}
```

Returns the full notification config JSON for the specified notification ID.

## Configuring via GitOps

Add notification templates to your `controls` section:

```yaml
controls:
  notifications:
    - path: ./notifications/restart-required.yml
    - path: ./notifications/firmware-update.yml
    - path: ./notifications/platform-sso.yml
```

Then run `fleetctl gitops` to deploy.

## Use cases

| Scenario | Heading | Primary action | Deferral |
| -------- | ------- | -------------- | -------- |
| Restart required | Restart Required | `url:ms-settings:windowsupdate` | `defer_1h` through `defer_1d` |
| Firmware update | Firmware Update Available | `url:https://internal.example.com/firmware` | `defer_4h` through `defer_7d` |
| Platform SSO enrollment | Platform SSO Required | `url:ms-settings:workplace` | `defer_1h` (max 3 defers) |
| MDM migration | Action Required: MDM Migration | `url:https://internal.example.com/mdm-migrate` | `defer_1d` (7d deadline) |
| Software install | New Software Available | `url:https://internal.example.com/install` | `defer_4h` with `watch_paths` |

<meta name="category" value="guides">
<meta name="authorGitHubUsername" value="TsekNet">
<meta name="authorFullName" value="Dan Tsekhanskiy">
<meta name="publishedOn" value="2026-03-05">
<meta name="articleTitle" value="Custom notifications">
<meta name="description" value="Send custom notifications to end users on macOS, Windows, and Linux hosts using Fleet.">
