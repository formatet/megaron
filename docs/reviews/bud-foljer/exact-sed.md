# Exact sed recipes

Use a separate clean checkout; restore each file before the next recipe. Commands byte-checked against the executed manifest substitutions.

## remote-meeting

```sh
sed -z -E -i 's#if\ pos\ ==\ at\ \{#if true {#' server/internal/messenger/order_pursuit.go
```

Expected assertion: `TestOrderPursuit_MovedUnitNeedsPhysicalLeg`.

## early-deadline

```sh
sed -z -E -i 's#if\ now\.Before\(deadline\)\ \{#if false \&\& now.Before(deadline) {#' server/internal/messenger/order_pursuit.go
```

Expected assertion: `TestOrderPursuit_GuardsAndWaiting/early`.

## stale-generation

```sh
sed -z -E -i 's#AND\ passage_generation=\$4#AND passage_generation>=$4#' server/internal/messenger/order_pursuit.go
```

Expected assertion: `TestOrderPursuit_GuardsAndWaiting/generation`.

## pending-witness

```sh
sed -z -E -i 's#e\.id>messengers\.carrier_witness_id#e.id<0 AND e.id>messengers.carrier_witness_id#' server/internal/messenger/order_pursuit.go
```

Expected assertion: `TestOrderPursuit_GuardsAndWaiting/witness`.

## physical-origin

```sh
sed -z -E -i 's#at,\ landTarget,\ now,\ arrival#province.MapPosition{Q:0,R:0}, landTarget, now, arrival#' server/internal/messenger/order_pursuit.go
```

Expected assertion: `TestOrderPursuit_MovedUnitNeedsPhysicalLeg`.

## temporary-wait

```sh
sed -z -E -i 's#p\.FollowWaiting\ =\ true#p.FollowWaiting = false#' server/internal/messenger/order_pursuit.go
```

Expected assertion: `TestOrderPursuit_GuardsAndWaiting/embarked`.

## owner-guard

```sh
sed -z -E -i 's#u\.OwnerID\ !=\ p\.PlayerID\ \|\|\ u\.WorldID\ !=\ p\.WorldID#false#' server/internal/messenger/order_pursuit.go
```

Expected assertion: `TestOrderPursuit_GuardsAndWaiting/owner`.

## moving-dispatch

```sh
sed -z -E -i 's#if\ p\.FollowUnit\ \{\n\	\	\	res,\ err\ =\ combat\.SetStanceInPursuit#if false \&\& p.FollowUnit {\n			res, err = combat.SetStanceInPursuit#' server/internal/messenger/order_delivery.go
```

Expected assertion: `TestOrderPursuit_MovingMeeting/stance`.

## moving-final-guard

```sh
sed -z -E -i 's#if\ o\.ExpectedPosition\ !=\ nil\ \&\&\ !\(redirecting\ \&\&\ o\.FollowingMeeting\)\ \{#if o.ExpectedPosition != nil {#' server/internal/combat/march_start.go
```

Expected assertion: `TestOrderPursuit_MovingMeeting/march`.

## legacy-contract

```sh
sed -z -E -i 's#!\(redirecting\ \&\&\ o\.FollowingMeeting\)#!redirecting#' server/internal/combat/march_start.go
```

Expected assertion: `TestOrderPursuit_UnflaggedMovingEnvelopeKeepsOldMeaning`.

## frozen-clock

```sh
sed -z -E -i 's#if\ route\.Version\ ==\ 2\ \&\&\ !route\.DepartureAt\.IsZero\(\)\ \&\&\ route\.TickNanoseconds\ >\ 0\ \{#if false \&\& route.Version == 2 \&\& !route.DepartureAt.IsZero() \&\& route.TickNanoseconds > 0 {#' server/internal/messenger/recall.go
```

Expected assertion: `TestInterceptCourierRoute_FrozenPhysicalClock`.

## keryx-json-field

```sh
sed -z -E -i 's#json:"following_unit"#json:"follow_unit"#' server/cmd/keryx/cmd_unit.go
```

Expected assertion: `TestOrdersEnRoutePursuitUsesNextMeeting`.

## keryx-pending

```sh
sed -z -E -i 's#if\ o\.FollowingUnit\ \{#if false \&\& o.FollowingUnit {#' server/cmd/keryx/cmd_unit_orders_en_route.go
```

Expected assertion: `TestOrdersEnRoutePursuitUsesNextMeeting`.

## web-runner-status

```sh
sed -z -E -i 's#if\ \(runner\.following_unit\)\ \{#if (false \&\& runner.following_unit) {#' web/static/js/megaron/ui/runner_status.js
```

Expected assertion: `order pursuit: next meeting time never promises delivery to a moving unit`.

## web-stance-receipt

```sh
sed -z -E -i 's#if\ \(d\?\.following_unit\)\ return#if (false \&\& d?.following_unit) return#' web/static/js/megaron/ui/stance.js
```

Expected assertion: `stance receipt: following-unit time is the next meeting, not guaranteed delivery`.

## web-march-receipt

```sh
sed -z -E -i 's#dispatched\ \&\&\ first\.data\.following_unit#false \&\& dispatched \&\& first.data.following_unit#' web/static/js/megaron/ui/marchctx.js
```

Expected assertion: `march receipt: a following Runner`.

## web-recall-redirect-receipt

```sh
sed -z -E -i 's#if\ \(d\?\.following_unit\)\ return#if (false \&\& d?.following_unit) return#' web/static/js/megaron/ui/drawers/war.js
```

Expected assertion: `recall and redirect receipts qualify the first meeting when the Runner follows`.

## api-opt-in

```sh
sed -z -E -i 's#payload\.FollowUnit\ =\ category\ ==\ string\(unit\.CategoryLand\)#payload.FollowUnit = false \&\& category == string(unit.CategoryLand)#' server/api/handlers/unit.go
```

Expected assertion: `TestOrderPursuitAPI_DispatchOptInPersists`.

## api-owner-metadata

```sh
sed -z -E -i 's#if\ m\.Own\ \&\&\ orderUnitID\ !=\ nil\ \{#if false \&\& m.Own \&\& orderUnitID != nil {#' server/api/handlers/world.go
```

Expected assertion: `TestOrderPursuitAPI_MapMetadataOnlyForOwner/owner`.
