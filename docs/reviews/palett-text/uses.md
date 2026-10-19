# Användningsinventering

Bas 07e2eac4; radnummer i slice 23. Text, ikoner och ytor blandas här; ingen automatisk omfärgning.

## --danger

```text
web/static/megaron.css:21: --danger:    #6C3483;   /* DOOM_VIOLET— collapse, danger states */
web/static/megaron.css:321: .btn-danger          { background: var(--bg-panel);  border-color: var(--danger);   color: var(--danger); }
web/static/megaron.css:322: .btn-danger:hover    { background: var(--danger);    border-color: var(--text);     color: var(--bg-panel); }
web/static/megaron.css:327: .form-error { color: var(--danger); font-size: var(--fs-sm); min-height: 1.2em; margin-bottom: 0.5rem; }
web/static/megaron.css:348: .state-collapsing { border-color: var(--danger);   color: var(--danger); }
web/static/megaron.css:468: .march-attack   .intent-tag, .intent-tag.attack    { border-color: var(--danger); color: var(--danger); }
web/static/megaron.css:557: .unit-unaffordable .recruit-cost { color:var(--danger); font-style:italic; }
web/static/megaron.css:622: .be-dir-in  { color:var(--danger); }
web/static/megaron.css:649: .ge-cat-divine_wrath .ge-text { color:var(--danger); }
web/static/megaron.css:650: .ge-cat-divine_wrath .ge-region { color:var(--danger); }
web/static/megaron.css:732: .mood-Wrathful   { color: var(--danger) !important; }
web/static/megaron.css:1087: background: var(--danger, #c0392b);
web/static/megaron.css:1439: .gubbe-hex.held { stroke: var(--danger); stroke-width: 2; stroke-dasharray: 3 2; }
web/static/megaron.css:1441: .gubbe-held-note { font-size: var(--fs-xs); color: var(--danger); margin: 0 0 0.3rem; }
web/static/megaron.css:1545: .notif-list-item.nl-urgent  { border-left: 2px solid var(--danger, #c0392b); }
web/static/megaron.css:1713: .food-state-starving  { color: var(--danger); }
web/static/js/megaron/ui/drawers/city.js:241: <span id="labor-save-err" style="font-size:.75rem;color:var(--danger)"></span>
web/static/js/megaron/ui/drawers/economy.js:107: const rateColor = r.grainRate < 0 ? 'var(--danger)' : (r.grainRate > 0 ? 'var(--safe-text)' : 'var(--text-dim)');
web/static/js/megaron/ui/drawers/economy.js:607: ? `<span style="color:var(--danger)">▼ ${fmtNum(Number(g.rate.toFixed(1)))} per day</span>`
web/static/js/megaron/ui/drawers/kult.js:40: const MOOD_COLORS = { Favorable:'var(--safe-text)', Indifferent:'var(--text-dim)', Suspicious:'var(--border)', Wrathful:'var(--danger)' };
```

## --accent-diplomacy

```text
web/static/megaron.css:31: --accent-diplomacy:  #7A4AA5;
web/static/megaron.css:972: .dc-diplomacy { border-left-color: var(--accent-diplomacy); background: rgba(122,74,165,0.13); box-shadow: var(--chip-shadow-diplomacy); }
web/static/megaron.css:996: .dc-diplomacy .dc-dot { background: var(--accent-diplomacy); }
web/static/js/megaron/ui/drawers/diplomacy.js:292: + '<span style="font-size:.82rem;font-weight:bold;color:var(--accent-diplomacy)">'
```

## --text-dim

```text
web/static/megaron.css:16: --text-dim:  #A04000;   /* LOAM       — muted / secondary text */
web/static/megaron.css:129: .nav-links a { color: var(--text-dim); font-size: var(--fs-sm); font-family: var(--mono); letter-spacing: 0.1em; }
web/static/megaron.css:137: color: var(--text-dim);
web/static/megaron.css:191: color: var(--text-dim);
web/static/megaron.css:249: color: var(--text-dim);
web/static/megaron.css:261: .field label { display: block; font-size: var(--fs-sm); color: var(--text-dim); margin-bottom: 0.3rem; text-transform: uppercase; letter-spacing: 0.1em; }
web/static/megaron.css:288: color: var(--text-dim);
web/static/megaron.css:319: .btn-secondary       { background: var(--bg-panel);  border-color: var(--text-dim); color: var(--border-in); }
web/static/megaron.css:335: .join-result { font-size: var(--fs-sm); margin-top: 0.4rem; color: var(--text-dim); }
web/static/megaron.css:346: .state-forming    { border-color: var(--text-dim); color: var(--text-dim); }
web/static/megaron.css:355: color: var(--text-dim);
web/static/megaron.css:377: .empty-state { color: var(--text-dim); font-style: italic; font-size: var(--fs-base); }
web/static/megaron.css:395: .world-meta { display: flex; gap: 0.8rem; font-size: var(--fs-sm); color: var(--text-dim); flex-wrap: wrap; margin-bottom: 0.4rem; }
web/static/megaron.css:396: .world-date { font-size: var(--fs-sm); color: var(--text-dim); }
web/static/megaron.css:441: .res-name  { font-size: var(--fs-xs); color: var(--text-dim); text-transform: uppercase; letter-spacing: 1px; font-family: var(--mono); }
web/static/megaron.css:455: .complete-at { color: var(--text-dim); font-family: var(--mono); font-size: var(--fs-sm); }
web/static/megaron.css:471: .arrives { color: var(--text-dim); font-size: var(--fs-sm); margin-left: 0.5rem; }
web/static/megaron.css:519: .sett-switch-link { background:var(--bg-panel); border:1px solid var(--border); padding:.2rem .7rem; font-size:var(--fs-sm); font-family:var(--mono); color:var(--text-dim); text-decoration:none; }
web/static/megaron.css:544: .army-dp-label { color:var(--text-dim); text-transform:uppercase; letter-spacing:.06em; }
web/static/megaron.css:551: .recruit-cost  { flex:1; font-size:var(--fs-sm); color:var(--text-dim); font-family:var(--mono); }
web/static/megaron.css:554: .unit-unavailable .recruit-cost { color:var(--text-dim); font-style:italic; }
web/static/megaron.css:569: .lp-title { font-size:var(--fs-sm); color:var(--text-dim); text-transform:uppercase; letter-spacing:.15em; margin-bottom:.3rem; font-family:var(--mono); }
web/static/megaron.css:572: .bldg-tag.l2 { background:var(--text-dim); }
web/static/megaron.css:587: .march-time   { color:var(--text-dim); font-size:var(--fs-sm); white-space:nowrap; }
web/static/megaron.css:596: .goods-table th { text-align:left; font-size:var(--fs-sm); color:var(--text-dim); text-transform:uppercase; letter-spacing:.08em; font-family:var(--mono); border-bottom:1px solid var(--border); padding:.15rem 0; }
web/static/megaron.css:601: .g-cat  { font-size:var(--fs-sm); color:var(--text-dim); font-family:var(--mono); }
web/static/megaron.css:605: .lp-label { font-size:var(--fs-sm); color:var(--text-dim); font-family:var(--mono); }
web/static/megaron.css:619: .be-when { font-size:var(--fs-sm); color:var(--text-dim); font-family:var(--mono); }
web/static/megaron.css:630: .tr-good { color:var(--text-dim); text-transform:uppercase; font-size:var(--fs-sm); letter-spacing:.06em; flex:1.2; }
web/static/megaron.css:633: .tr-arrow { color:var(--text-dim); }
web/static/megaron.css:638: .recipe-line { font-size:var(--fs-sm); font-family:var(--mono); color:var(--text-dim); margin:.2rem 0; }
web/static/megaron.css:644: .ge-region { display:block; font-size:var(--fs-sm); text-transform:uppercase; letter-spacing:.07em; color:var(--text-dim); }
web/static/megaron.css:646: .ge-time { display:block; font-size:var(--fs-sm); color:var(--text-dim); }
web/static/megaron.css:662: .no-kingdom p { color: var(--text-dim); margin-bottom: 1.5rem; }
web/static/megaron.css:667: .dispossessed-page p { color: var(--text-dim); margin-bottom: 2rem; }
web/static/megaron.css:670: .loading { color: var(--text-dim); font-style: italic; font-size: var(--fs-sm); }
web/static/megaron.css:678: .wanax-table th { text-align: left; padding: .4rem .6rem; background: var(--bg-panel); border-bottom: 2px solid var(--border); color: var(--text-dim); font-size: var(--fs-sm); text-transform: uppercase; letter-spacing: .05em; }
web/static/megaron.css:684: .wanax-table .dim { color: var(--text-dim); }
web/static/megaron.css:689: .tr-bal-label { font-size:var(--fs-sm); text-transform:uppercase; letter-spacing:.12em; color:var(--text-dim); font-family:var(--mono); }
web/static/megaron.css:691: .tr-tribute   { font-size:var(--fs-sm); color:var(--text-dim); font-family:var(--mono); }
web/static/megaron.css:714: .mh-world { font-size: var(--fs-sm); font-family: var(--mono); color: var(--text-dim); letter-spacing: .1em; }
web/static/megaron.css:725: .mhs-label { font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: .12em; color: var(--text-dim); font-family: var(--mono); }
web/static/megaron.css:770: color: var(--text-dim);
web/static/megaron.css:788: color: var(--text-dim);
web/static/megaron.css:793: .rs-dim   { color: var(--text-dim); font-style: italic; }
web/static/megaron.css:799: color: var(--text-dim);
web/static/megaron.css:820: color: var(--text-dim);
web/static/megaron.css:856: color: var(--text-dim);
web/static/megaron.css:940: color: var(--text-dim);
web/static/megaron.css:1051: font-size: var(--fs-xs); color: var(--text-dim);
web/static/megaron.css:1059: color: var(--text-dim);
web/static/megaron.css:1074: color: var(--text-dim);
web/static/megaron.css:1174: color: var(--text-dim);
web/static/megaron.css:1261: color: var(--text-dim);
web/static/megaron.css:1274: color: var(--text-dim);
web/static/megaron.css:1282: .drawer-nav { background: none; border: 1px solid var(--border); color: var(--text-dim); cursor: pointer; padding: 0.05rem 0.4rem; font-family: var(--mono); font-size: var(--fs-sm); }
web/static/megaron.css:1296: color: var(--text-dim);
web/static/megaron.css:1344: .retreat-note { font-size: var(--fs-sm); color: var(--text-dim); margin: 0.3rem 0 0; }
web/static/megaron.css:1345: .retreat-res { font-size: var(--fs-sm); color: var(--text-dim); min-height: 1rem; margin-top: 0.2rem; }
web/static/megaron.css:1351: color: var(--text-dim);
web/static/megaron.css:1376: .obj-sub   { font-size: var(--fs-sm); color: var(--text-dim); font-family: var(--mono); }
web/static/megaron.css:1389: .rel-neutral { color: var(--text-dim); border-color: var(--text-dim); }
web/static/megaron.css:1415: .sr-label { color: var(--text-dim); font-family: var(--mono); font-size: var(--fs-sm); }
web/static/megaron.css:1428: .goods-mini td.goods-atcap { color: var(--text-dim); }
web/static/megaron.css:1446: .gubbe-hex-sub { font-family: var(--mono); font-size: 7px; fill: var(--text-dim); text-anchor: middle; pointer-events: none; }
web/static/megaron.css:1447: .gubbe-fow-list { font-size: var(--fs-xs); color: var(--text-dim); margin-top: 0.3rem; }
web/static/megaron.css:1462: .ii-from { font-size: var(--fs-sm); font-family: var(--mono); color: var(--text-dim); margin-bottom: 0.15rem; }
web/static/megaron.css:1471: .gossip-hops { color: var(--text-dim); font-style: normal; }
web/static/megaron.css:1482: .growth-line { color: var(--text-dim); font-size: var(--fs-sm); margin: 0.3rem 0; }
web/static/megaron.css:1494: .sb-label { font-size: var(--fs-sm); text-transform: uppercase; letter-spacing: 0.12em; color: var(--text-dim); font-family: var(--mono); }
web/static/megaron.css:1501: color: var(--text-dim);
web/static/megaron.css:1514: color: var(--text-dim);
web/static/megaron.css:1548: .nli-time { color: var(--text-dim); font-family: var(--mono); font-size: var(--fs-sm); white-space: nowrap; }
web/static/megaron.css:1549: .nli-grain { grid-column: 2 / -1; color: var(--text-dim); font-size: var(--fs-sm); }
web/static/megaron.css:1576: .si-glyph  { padding: 0 0.8rem; color: var(--text-dim); font-size: var(--fs-base); flex-shrink: 0; }
web/static/megaron.css:1583: background: none; color: var(--text-dim);
web/static/megaron.css:1589: color: var(--text-dim); font-family: var(--mono);
web/static/megaron.css:1626: .dw-grain { margin-top: 0.4rem; color: var(--text-dim); font-size: var(--fs-sm); }
web/static/megaron.css:1627: .dw-time { margin-top: 0.3rem; color: var(--text-dim); font-family: var(--mono); font-size: var(--fs-xs); }
web/static/megaron.css:1650: .dw-occ-label { color: var(--text-dim); font-size: var(--fs-xs); margin-bottom: 0.4rem; }
web/static/megaron.css:1664: .dw-occ-desc { color: var(--text-dim); font-size: var(--fs-xs); }
web/static/megaron.css:1667: .dw-occ-result { margin-top: 0.4rem; color: var(--text-dim); font-size: var(--fs-xs); font-family: var(--mono); }
web/static/megaron.css:1677: color: var(--text-dim);
web/static/megaron.css:1689: .sr-meta { font-size: var(--fs-sm); color: var(--text-dim); font-family: var(--mono); }
web/static/megaron.css:1690: .sr-type { font-size: var(--fs-xs); padding: 0.03rem 0.3rem; border: 1px solid var(--border); color: var(--text-dim); flex-shrink: 0; }
web/static/megaron.css:1711: .food-state-stable, .food-state-unknown { color: var(--text-dim); }
web/static/megaron.css:1718: color: var(--text-dim); font-family: var(--mono); background: var(--bg);
web/static/megaron.css:1737: color: var(--text-dim);
web/static/megaron.css:1744: color: var(--text-dim);
web/static/megaron.css:1790: color: var(--text-dim);
web/static/megaron.css:1855: color: var(--text-dim);
web/static/megaron.css:1871: color: var(--text-dim);
web/static/megaron.css:1910: color: var(--text-dim);
web/static/megaron.css:1943: color: var(--text-dim);
web/static/megaron.css:1951: .cx-link-missing { color: var(--text-dim); }
web/static/megaron.css:1957: color: var(--text-dim);
web/static/megaron.css:1996: color: var(--text-dim);
web/static/megaron.css:2008: .alpha-label { margin: 0 0 .25rem; font-size: var(--fs-sm); color: var(--text-dim); letter-spacing: .04em; }
web/static/megaron.css:2011: .alpha-info h2 { margin: 0 0 .75rem; font-size: var(--fs-md); font-weight: 500; color: var(--text); padding-top: 1rem; border-top: 1px solid var(--text-dim); }
web/static/megaron.css:2018: .alpha-note { font-size: var(--fs-sm); color: var(--text-dim); }
web/static/megaron.css:2020: .alpha-info img { display: block; width: 100%; height: auto; border: 1px solid var(--text-dim); }
web/static/megaron.css:2021: .alpha-info figcaption { margin-top: .4rem; font-size: var(--fs-sm); color: var(--text-dim); }
web/static/megaron.css:2022: .alpha-footer { padding-top: 1rem; border-top: 1px solid var(--text-dim); }
web/static/megaron.css:2029: .account-chat dt { font-size: var(--fs-sm); color: var(--text-dim); }
web/static/megaron.css:2033: .account-delete-note { margin: 1rem 0; font-size: var(--fs-sm); color: var(--text-dim); }
web/static/js/megaron/render/map.js:3288: + '<span>' + lbl + ' <span style="color:var(--text-dim)">(' + u.status + ')</span></span>'
web/static/js/megaron/render/map.js:3299: + '<span>' + lbl + ' ×' + u.size + ' <span style="color:var(--text-dim)">(' + owner + ', ' + u.status + ')</span></span>'
web/static/js/megaron/ui/citygrid.js:78: : `<span style="color:var(--text-dim)">${fmtNum(good.placed)} placed, uncapped</span>`;
web/static/js/megaron/ui/citygrid.js:85: <span style="color:var(--text-dim)"> ${fmtNum(Number(good.rate_per_tick.toFixed(1)))} per day total · ${nextLabel}</span>
web/static/js/megaron/ui/citygrid.js:294: <span style="color:var(--text-dim)">${fmtNum(opts.pool_size)} free</span>
web/static/js/megaron/ui/drawers/city.js:227: <span class="sr-label">Devotion <span style="color:var(--text-dim);font-size:.65rem">(cult, additive)</span></span>
web/static/js/megaron/ui/drawers/city.js:234: <div style="font-size:.72rem;color:var(--text-dim)"><span class="labor-cit" data-good="cult">${fmtNum(Math.round(devWeight*lp))}</span> workers · <span id="labor-rate-cult">${fmtNum(devPct)} percent of ${fmtNum(devCapPct)} percent capacity${atCap ? ' · at cap — build a higher-level temple to devote more' : ''}</span></div>
web/static/js/megaron/ui/drawers/city.js:274: <tr style="color:var(--text-dim);font-size:.7rem"><td>Unit</td><td style="text-align:right">Count</td><td style="text-align:right">Pop</td><td style="text-align:right">DP</td></tr>
web/static/js/megaron/ui/drawers/city.js:277: return `<tr><td>${unitTypeLabel(k)}</td><td style="text-align:right">${n}</td><td style="text-align:right;color:var(--text-dim)">${n*POP_COSTS[k]}</td><td style="text-align:right;color:var(--safe-text)">${n*UNIT_DP[k]}</td></tr>`;
web/static/js/megaron/ui/drawers/city.js:534: <tr style="color:var(--text-dim);font-size:.7rem"><td>Day</td><td>Production</td><td>Consumption</td><td>Events</td></tr>
web/static/js/megaron/ui/drawers/city.js:539: return `<tr><td>#${fmtNum(t.tick)}</td><td style="color:var(--safe-text)">${prod}</td><td style="color:var(--accent)">${cons}</td><td style="color:var(--text-dim)">${evs}</td></tr>`;
web/static/js/megaron/ui/drawers/city.js:640: <div style="font-size:.72rem;color:var(--text-dim);margin-bottom:.3rem">${ingredientsStr} → ${foundryRecipe.output_qty} ${foundryRecipe.output_key} · stock: ${stockStr}</div>
web/static/js/megaron/ui/drawers/diplomacy.js:295: + '<span style="font-size:.68rem;color:var(--text-dim)">'
web/static/js/megaron/ui/drawers/diplomacy.js:320: const countdown = m.expires_at ? '<div style="font-size:.68rem;color:var(--text-dim)">' + expiryWord(m.expires_at) + '</div>' : '';
web/static/js/megaron/ui/drawers/diplomacy.js:340: tradeHtml = '<div style="font-size:.72rem;color:var(--text-dim);font-style:italic;margin:.2rem 0">Trade: ' + esc(offer.status) + esc(tradeScheduleText(offer)) + '</div>';
web/static/js/megaron/ui/drawers/diplomacy.js:363: : offerStatus === 'declined' ? '<span style="color:var(--text-dim)">✗ declined</span>'
web/static/js/megaron/ui/drawers/diplomacy.js:364: : offerStatus === 'expired'  ? '<span style="color:var(--text-dim)">⏳ expired</span>'
web/static/js/megaron/ui/drawers/diplomacy.js:371: const countdown = m.expires_at ? '<div style="font-size:.68rem;color:var(--text-dim);text-align:right">' + expiryWord(m.expires_at) + '</div>' : '';
web/static/js/megaron/ui/drawers/diplomacy.js:390: passageBtn = '<div style="font-size:.68rem;color:var(--text-dim);font-style:italic;margin-top:.3rem;text-align:right">no ship of yours is free there yet</div>';
web/static/js/megaron/ui/drawers/diplomacy.js:427: + '<div><div style="font-size:.68rem;color:var(--text-dim)">Want good</div><select id="' + cid + '-good"' + goodsSelectDisabledAttr(tradeableGoods) + '>' + goodsOptionsHTML(tradeableGoods) + '</select></div>'
web/static/js/megaron/ui/drawers/diplomacy.js:428: + '<div><div style="font-size:.68rem;color:var(--text-dim)">Quantity</div><input id="' + cid + '-qty" type="number" min="0.1" step="0.1" placeholder="50"></div>'
web/static/js/megaron/ui/drawers/diplomacy.js:429: + '<div><div style="font-size:.68rem;color:var(--text-dim)">Offer silver</div><input id="' + cid + '-silver" type="number" min="1" step="1" placeholder="60"></div>'
web/static/js/megaron/ui/drawers/diplomacy.js:432: + '<div><div style="font-size:.68rem;color:var(--text-dim)">Offer good</div><select id="' + cid + '-offer-good"' + goodsSelectDisabledAttr(tradeableGoods) + '>' + goodsOptionsHTML(tradeableGoods) + '</select></div>'
web/static/js/megaron/ui/drawers/diplomacy.js:433: + '<div><div style="font-size:.68rem;color:var(--text-dim)">Quantity</div><input id="' + cid + '-offer-qty" type="number" min="0.1" step="0.1" placeholder="20"></div>'
web/static/js/megaron/ui/drawers/diplomacy.js:434: + '<div><div style="font-size:.68rem;color:var(--text-dim)">Want silver</div><input id="' + cid + '-want-silver" type="number" min="1" step="1" placeholder="80"></div>'
web/static/js/megaron/ui/drawers/diplomacy.js:568: if (res.ok && block) block.innerHTML = '<span style="color:var(--text-dim);font-style:italic">Declined.</span>';
web/static/js/megaron/ui/drawers/diplomacy.js:619: if (row) row.outerHTML = '<div style="font-size:.72rem;color:var(--text-dim);margin-top:.3rem;text-align:right">↩ Called back — returns ' + arrivalHTML(data.returns_at) + ', undelivered</div>';
web/static/js/megaron/ui/drawers/economy.js:99: <tr style="color:var(--text-dim);font-size:.7rem">
web/static/js/megaron/ui/drawers/economy.js:107: const rateColor = r.grainRate < 0 ? 'var(--danger)' : (r.grainRate > 0 ? 'var(--safe-text)' : 'var(--text-dim)');
web/static/js/megaron/ui/drawers/economy.js:226: html += `<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem"><td>Good</td><td>Amount</td><td>Rate</td></tr>${others.map(g =>
web/static/js/megaron/ui/drawers/economy.js:304: return '<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem">' +
web/static/js/megaron/ui/drawers/economy.js:311: '<p style="color:var(--text-dim);font-size:.68rem;margin-top:.3rem">' +
web/static/js/megaron/ui/drawers/economy.js:518: return '<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem">' +
web/static/js/megaron/ui/drawers/economy.js:528: const shipNote = o.ship_name ? ` <span style="color:var(--text-dim)">⛵ ${esc(o.ship_name)}</span>` : '';
web/static/js/megaron/ui/drawers/economy.js:613: <div class="dsec-title" style="font-size:.75rem">${esc(sw.name)}${sw.secondhand ? ' <span style="color:var(--text-dim);font-size:.68rem">(rumour)</span>' : ''}</div>
web/static/js/megaron/ui/drawers/economy.js:622: <div class="dsec-title" style="font-size:.75rem">${esc(sw.name)}${sw.secondhand ? ' <span style="color:var(--text-dim);font-size:.68rem">(rumour)</span>' : ''}</div>
web/static/js/megaron/ui/drawers/kult.js:40: const MOOD_COLORS = { Favorable:'var(--safe-text)', Indifferent:'var(--text-dim)', Suspicious:'var(--border)', Wrathful:'var(--danger)' };
web/static/js/megaron/ui/drawers/kult.js:78: <span class="sr-label">${esc(p.name)} <span style="color:var(--text-dim);font-size:.68rem">(${esc(p.god)}, ≥${p.min_kharis} kharis)</span><br>
web/static/js/megaron/ui/drawers/kult.js:79: <span style="color:var(--text-dim);font-size:.7rem">${esc(p.effect || '')}<br>traditional offering: ${offeringStr}</span></span>
web/static/js/megaron/ui/drawers/kult.js:173: return `<span style="color:var(--text-dim);font-size:.72rem">on cooldown — ${esc(fmtDays(waitDays))}</span>`;
web/static/js/megaron/ui/drawers/kult.js:176: return '<span style="color:var(--text-dim);font-size:.72rem">not affordable</span>';
web/static/js/megaron/ui/drawers/notif.js:108: html += `<div class="notif-list-item" style="cursor:pointer;color:var(--text-dim)" onclick="notifShowKind()">
web/static/js/megaron/ui/drawers/notif.js:144: html += `<div class="notif-list-item" style="cursor:pointer;color:var(--text-dim)" onclick="notifShowKind('${kind}')">
web/static/js/megaron/ui/drawers/war.js:217: recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.25rem 0;font-size:.78rem;color:var(--text-dim)">'
web/static/js/megaron/ui/drawers/war.js:225: recHtml += '<div style="font-size:.65rem;color:var(--text-dim);margin-bottom:.3rem">Land units train as a full 100-man cohort. A cohort worn down in battle regenerates over time in its home city — garrison it there and use Reinforce (Army tab). Ships are built one at a time.</div>';
web/static/js/megaron/ui/drawers/war.js:245: + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
web/static/js/megaron/ui/drawers/war.js:254: + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
web/static/js/megaron/ui/drawers/war.js:364: resEl.style.color = 'var(--text-dim)';
web/static/js/megaron/ui/drawers/war.js:388: resEl.style.color = 'var(--text-dim)';
web/static/js/megaron/ui/drawers/war.js:491: const dim = (txt) => '<span style="font-size:.65rem;color:var(--text-dim)">' + txt + '</span>';
web/static/js/megaron/ui/drawers/war.js:517: ? '<div class="unit-upkeep" style="font-size:.65rem;color:var(--text-dim)">' + upkeepLabel + ': '
web/static/js/megaron/ui/drawers/war.js:529: ? '<span style="font-size:.6rem;padding:.1rem .25rem;border:1px solid var(--border);color:var(--text-dim);margin-left:.2rem">' + esc(u.stance) + '</span>'
web/static/js/megaron/ui/drawers/war.js:534: ? '<span style="font-size:.6rem;color:var(--text-dim);margin-left:.3rem">crew ' + fmtNum(u.crew) + '</span>'
web/static/js/megaron/ui/drawers/war.js:559: (days <= 0 ? 'var(--accent-war)' : days < 3 ? 'var(--accent-war)' : 'var(--text-dim)') + '">' +
web/static/js/megaron/ui/drawers/war.js:644: pickupRow = '<div id="upick-' + u.id + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
web/static/js/megaron/ui/drawers/war.js:687: redirectRow = '<div id="uredir-' + u.id + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
web/static/js/megaron/ui/drawers/war.js:689: + '<div style="margin-top:.2rem"><a href="#" onclick="unitRedirectTypedToggle(\'' + u.id + '\');return false" style="color:var(--text-dim)">or type coordinates</a></div>'
web/static/js/megaron/ui/drawers/war.js:697: redirectRow = '<div style="margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">On its mission — returns to port automatically, no orders reach it at sea.</div>';
web/static/js/megaron/ui/drawers/war.js:699: const orderStatus = '<div id="uorder-' + u.id + '" style="font-size:.65rem;color:var(--text-dim);margin-top:.15rem"></div>';
web/static/js/megaron/ui/drawers/war.js:705: + '<span style="font-size:.68rem;color:var(--text-dim);margin-left:auto">' + esc(unitStatusLabel(u)) + '</span>'
web/static/js/megaron/ui/drawers/war.js:709: + (loc ? '<div style="font-size:.65rem;color:var(--text-dim)">' + loc + '</div>' : '')
web/static/js/megaron/ui/drawers/war.js:710: + (u.expedition ? '<div style="font-size:.72rem;color:var(--text-dim)">' + esc(expeditionMissionText(u.expedition)) + '</div>' : '')
web/static/js/megaron/ui/drawers/war.js:836: resEl.style.color = 'var(--text-dim)';
web/static/js/megaron/ui/drawers/war.js:869: resEl.style.color = 'var(--text-dim)';
web/static/js/megaron/ui/marchctx.js:135: prevEl.innerHTML = '<span style="color:var(--text-dim)">Reading the land…</span>';
web/static/js/megaron/ui/marchctx.js:144: if (State.marchCtxDest === dest) prevEl.innerHTML = '<span style="color:var(--text-dim)">No forecast available.</span>';
web/static/js/megaron/ui/marchctx.js:170: let html = `<div style="color:var(--text-dim)">Catchment forecast — ${fmtNum(known)} of ${fmtNum(total)} hexes known</div>`;
web/static/js/megaron/ui/marchctx.js:180: html += `<div style="color:var(--text-dim)">A colony does not feed itself — build a farm if the land bears it, or send grain by internal transfer.</div>`;
web/static/js/megaron/ui/marchctx.js:199: if (extras.length) html += `<div style="color:var(--text-dim)">Also: ${extras.join(', ')}</div>`;
web/static/js/megaron/ui/marchctx.js:206: html += `<div>🎁 <b>${gift.label}</b> <span style="color:var(--text-dim)">— ${gift.detail}</span></div>`;
web/static/js/megaron/ui/marchctx.js:218: const centerTag = i === 0 ? ' <span style="color:var(--text-dim)">(center)</span>' : '';
web/static/js/megaron/ui/marchctx.js:219: if (!ce.known) return `<div>? <span style="color:var(--text-dim)">unexplored</span>${centerTag}</div>`;
web/static/js/megaron/ui/marchctx.js:225: const depStr = hexDeps.length ? ` <span style="color:var(--text-dim)">· ${hexDeps.join(', ')}</span>` : '';
web/static/js/megaron/ui/marchctx.js:386: document.getElementById('mctx-units').innerHTML = '<span style="color:var(--text-dim);font-size:.75rem">Loading units…</span>';
web/static/js/megaron/ui/marchctx.js:505: el.innerHTML = '<p style="font-size:.73rem;color:var(--text-dim);margin:.3rem 0">' + empty + '</p>';
web/static/js/megaron/ui/roadorder.js:47: return '<div id="uroad-' + id + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
web/static/js/megaron/ui/roadorder.js:52: return '<div id="uroad-' + id + '" data-from="' + from.q + ',' + from.r + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
web/static/js/megaron/ui/runner_status.js:19: const dim = s => '<span style="color:var(--text-dim)">' + s + '</span>';
web/static/js/megaron/ui/runner_status.js:57: const line = s => '<div style="font-size:.65rem;color:var(--text-dim)">🏃 ' + s + '</div>';
web/static/js/megaron/ui/search.js:156: if (!html) html = '<div style="padding:.6rem .8rem;font-size:.8rem;color:var(--text-dim)">No results.</div>';
web/templates/join.html:24: .join-players { font-family:var(--mono); font-size:var(--fs-sm); color:var(--text-dim); }
```
