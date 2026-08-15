'use client';

import React, { useEffect, useMemo, useRef, useState } from 'react';
// A static import, not import('maplibre-gl').then(...). The dynamic form made
// map creation asynchronous, which under React StrictMode's dev-only double
// effect invoke raced two import() calls against the synchronous mount/cleanup
// dance and left the map in whichever state the two callbacks happened to
// interleave into. A synchronous import has nothing to race.
// A namespace import, not a default one: v6 dropped the default export in
// favour of named ones (Map, Popup, NavigationControl, ...), so `import
// maplibregl from 'maplibre-gl'` no longer type-checks.
import * as maplibregl from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import type { FeatureCollection, Geometry } from 'geojson';

// MapLibre works out its own worker script's URL from import.meta.url inside
// its bundled internals, which Turbopack rewrites and no longer points at a
// servable file. Left alone, that resolves to an empty string, and the
// browser tries to run the current page's HTML as a JS module in its place.
// new URL(..., import.meta.url) written here, in application code Next.js's
// bundler actually tracks, gets the real file and a URL it will serve.
if (typeof window !== 'undefined') {
  maplibregl.setWorkerUrl(
    new URL('maplibre-gl/dist/maplibre-gl-worker.mjs', import.meta.url).toString(),
  );
}

const STYLE = 'https://tiles.openfreemap.org/styles/liberty';
const STYLE_HOST = 'tiles.openfreemap.org';

// The basemap and every feature layer only ever get drawn from inside the
// map's 'load' handler, so a style fetch that fails takes both down at once
// and, without this, silently: the container just stays blank forever.
const LOAD_TIMEOUT_MS = 12000;

export interface GeoMapPreviewProps {
  data?: unknown;
  tileUrl?: string;
  className?: string;
  onNavigateLink?: (href: string) => void;
}

function extendBoundsFromGeometry(bounds: any, geometry: Geometry) {
  const extend = (coords: number[]) => {
    if (
      typeof coords[0] === 'number' &&
      typeof coords[1] === 'number' &&
      !isNaN(coords[0]) &&
      !isNaN(coords[1])
    ) {
      bounds.extend([coords[0], coords[1]] as [number, number]);
    }
  };

  const walk = (g: Geometry) => {
    if (!g) return;
    if (g.type === 'Point') extend(g.coordinates);
    else if (g.type === 'MultiPoint' || g.type === 'LineString')
      g.coordinates.forEach(extend);
    else if (g.type === 'MultiLineString' || g.type === 'Polygon')
      g.coordinates.forEach((ring) => ring.forEach(extend));
    else if (g.type === 'MultiPolygon')
      g.coordinates.forEach((poly) =>
        poly.forEach((ring) => ring.forEach(extend)),
      );
    else if (g.type === 'GeometryCollection') g.geometries.forEach(walk);
  };

  walk(geometry);
}

function ensureLayers(map: any) {
  if (!map.getLayer('features-fill')) {
    map.addLayer({
      id: 'features-fill',
      type: 'fill',
      source: 'features',
      paint: { 'fill-color': '#e63946', 'fill-opacity': 0.35 },
    });
  }
  if (!map.getLayer('features-outline')) {
    map.addLayer({
      id: 'features-outline',
      type: 'line',
      source: 'features',
      paint: { 'line-color': '#1d3557', 'line-width': 1.5 },
    });
  }
  if (!map.getLayer('features-line')) {
    map.addLayer({
      id: 'features-line',
      type: 'line',
      source: 'features',
      layout: { 'line-join': 'round', 'line-cap': 'round' },
      paint: { 'line-color': '#457b9d', 'line-width': 2.5 },
    });
  }
  if (!map.getLayer('features-circle')) {
    map.addLayer({
      id: 'features-circle',
      type: 'circle',
      source: 'features',
      paint: {
        'circle-radius': 7,
        'circle-color': '#e63946',
        'circle-stroke-width': 2,
        'circle-stroke-color': '#ffffff',
      },
    });
  }
}

function checkObj(obj: any): FeatureCollection | null {
  if (!obj || typeof obj !== 'object') return null;
  if (obj.type === 'FeatureCollection' && Array.isArray(obj.features)) {
    return obj;
  }
  if (obj.type === 'Feature' && obj.geometry) {
    return { type: 'FeatureCollection', features: [obj] };
  }
  if (Array.isArray(obj) && obj[0]?.geometry) {
    return { type: 'FeatureCollection', features: obj };
  }
  if (obj.extent?.spatial?.bbox) {
    const rawBbox = obj.extent.spatial.bbox;
    const b = Array.isArray(rawBbox[0]) ? rawBbox[0] : rawBbox;
    if (b && b.length >= 4) {
      return {
        type: 'FeatureCollection',
        features: [
          {
            type: 'Feature',
            id: obj.id,
            properties: { title: obj.title || obj.id || 'Collection Extent', ...obj.summaries },
            geometry: {
              type: 'Polygon',
              coordinates: [
                [
                  [b[0], b[1]],
                  [b[2], b[1]],
                  [b[2], b[3]],
                  [b[0], b[3]],
                  [b[0], b[1]],
                ],
              ],
            },
          },
        ],
      };
    }
  }
  if (obj.bbox && Array.isArray(obj.bbox) && obj.bbox.length >= 4) {
    const b = obj.bbox;
    return {
      type: 'FeatureCollection',
      features: [
        {
          type: 'Feature',
          id: obj.id,
          properties: { title: obj.title || obj.id || 'Spatial Bounding Box' },
          geometry: {
            type: 'Polygon',
            coordinates: [
              [
                [b[0], b[1]],
                [b[2], b[1]],
                [b[2], b[3]],
                [b[0], b[3]],
                [b[0], b[1]],
              ],
            ],
          },
        },
      ],
    };
  }
  return null;
}

export function GeoMapPreview({
  data,
  tileUrl,
  className = '',
  onNavigateLink,
}: GeoMapPreviewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<any>(null);
  const loadTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [basemapError, setBasemapError] = useState<string | null>(null);

  const geojson = useMemo<FeatureCollection | null>(() => {
    if (!data) return null;
    try {
      const parsed = typeof data === 'string' ? JSON.parse(data) : data;
      return checkObj(parsed);
    } catch {
      return null;
    }
  }, [data]);

  const featureCount = useMemo(() => geojson?.features?.length ?? null, [geojson]);

  const detectedTypes = useMemo(() => {
    const types = new Set<string>();
    if (geojson?.features) {
      geojson.features.forEach((f) => {
        if (f.geometry?.type) types.add(f.geometry.type);
      });
    }
    return Array.from(types);
  }, [geojson]);

  useEffect(() => {
    if (!containerRef.current) return;

    function applyData(map: any) {
      // 1. Raster tiles
      if (tileUrl) {
        if (map.getLayer('ogc-raster-layer')) map.removeLayer('ogc-raster-layer');
        if (map.getSource('ogc-raster')) map.removeSource('ogc-raster');
        map.addSource('ogc-raster', {
          type: 'raster',
          tiles: [tileUrl],
          tileSize: 256,
        });
        map.addLayer({
          id: 'ogc-raster-layer',
          type: 'raster',
          source: 'ogc-raster',
          paint: { 'raster-opacity': 0.95 },
        });
      }

      // 2. GeoJSON features
      if (geojson && geojson.features && geojson.features.length > 0) {
        if (map.getSource('features')) {
          map.getSource('features').setData(geojson);
        } else {
          map.addSource('features', { type: 'geojson', data: geojson });
        }
        ensureLayers(map);

        const bounds = new maplibregl.LngLatBounds();
        geojson.features.forEach((f) => {
          if (f.geometry) extendBoundsFromGeometry(bounds, f.geometry);
        });

        if (!bounds.isEmpty()) {
          map.fitBounds(bounds, { padding: 48, maxZoom: 16 });
        }
        setTimeout(() => map.resize(), 100);

        // Popup details on click
        const clickable = ['features-circle', 'features-fill', 'features-line'];
        clickable.forEach((layerId) => {
          map.off('click', layerId);
          map.on('click', layerId, (e: any) => {
            if (!e.features || e.features.length === 0) return;
            const f = e.features[0];
            const props = f.properties || {};

            const html = `
              <div style="font-family: system-ui, sans-serif; font-size: 11px; max-height: 220px; overflow-y: auto; color: #0f172a; padding: 2px;">
                <div style="font-weight: 700; margin-bottom: 4px; border-bottom: 1px solid #e2e8f0; padding-bottom: 2px;">
                  Feature ${f.id ? `#${f.id}` : ''}
                </div>
                <pre style="margin: 0; white-space: pre-wrap; font-family: monospace; font-size: 10px; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 4px; padding: 4px; color: #334155;">${JSON.stringify(props, null, 2)}</pre>
              </div>
            `;

            new maplibregl.Popup({ closeButton: true, maxWidth: '280px' })
              .setLngLat(e.lngLat)
              .setHTML(html)
              .addTo(map);
          });
        });
      }
    }

    // Map already exists (a data or tileUrl change, not the first mount):
    // update it in place rather than tearing it down and rebuilding.
    if (mapRef.current) {
      const map = mapRef.current;
      if (map.isStyleLoaded()) {
        applyData(map);
      } else {
        map.once('load', () => applyData(map));
      }
      return;
    }

    // First mount: build the map. This runs synchronously within the effect,
    // so React's StrictMode dev double-invoke (setup, cleanup, setup again)
    // creates a throwaway map on the first pass, destroys it via the cleanup
    // effect below, then creates the one that stays — no async gap for two
    // in-flight creations to collide in.
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: STYLE,
      center: [100.5018, 13.7563],
      zoom: 5,
    });

    mapRef.current = map;
    setBasemapError(null);

    map.addControl(
      new maplibregl.NavigationControl({ showCompass: true, showZoom: true }),
      'top-right',
    );
    map.addControl(
      new maplibregl.AttributionControl({ compact: true }),
      'bottom-right',
    );

    // A failed style fetch (blocked host, no network) fires 'error' rather
    // than 'load', so this is the only way the container ever explains why
    // it stayed blank. But MapLibre also fires 'error' for plenty of things
    // that are not that: one missing sprite icon, one glyph range that 404s,
    // both routine and both harmless to the map underneath. The distinction
    // that matters is whether the style was ever received at all — once
    // 'styledata' has fired once, the fetch succeeded, and no error after
    // that point is reason to hide a map that is otherwise fine.
    let styleReceived = false;
    map.once('styledata', () => {
      styleReceived = true;
    });
    map.on('error', (e: any) => {
      if (styleReceived) return; // a rendering-time error, not a style-load one
      setBasemapError(e?.error?.message || `Could not load the basemap from ${STYLE_HOST}.`);
    });

    loadTimeoutRef.current = setTimeout(() => {
      if (mapRef.current === map && !map.loaded()) {
        setBasemapError(`Timed out waiting for the basemap from ${STYLE_HOST}.`);
      }
    }, LOAD_TIMEOUT_MS);

    map.on('load', () => {
      if (loadTimeoutRef.current) clearTimeout(loadTimeoutRef.current);
      setBasemapError(null);
      applyData(map);
    });
  }, [geojson, tileUrl]);

  useEffect(() => {
    return () => {
      if (loadTimeoutRef.current) clearTimeout(loadTimeoutRef.current);
      if (mapRef.current) {
        mapRef.current.remove();
        mapRef.current = null;
      }
    };
  }, []);

  return (
    <div className={`relative flex flex-col overflow-hidden rounded-control border border-rule bg-paper shadow-xs ${className}`}>
      <div className="flex items-center justify-between border-b border-rule bg-paper-2 px-3 py-1.5 text-xs text-muted">
        <div className="flex items-center gap-2">
          {tileUrl ? (
            <>
              <span className="font-semibold text-accent flex items-center gap-1.5">
                🗺️ Live Raster Tiles Active
              </span>
              <span className="rounded-chip bg-accent-wash px-1.5 py-0.2 font-mono text-[9px] font-medium text-accent border border-accent-edge">
                XYZ / EPSG:3857
              </span>
            </>
          ) : featureCount !== null ? (
            <span>
              Rendered <strong>{featureCount}</strong> feature{featureCount === 1 ? '' : 's'} on MapLibre
            </span>
          ) : (
            <span>Map Preview</span>
          )}

          {detectedTypes.length > 0 && (
            <div className="flex gap-1">
              {detectedTypes.map((t) => (
                <span
                  key={t}
                  className="rounded-chip bg-accent-wash px-1.5 py-0.2 font-mono text-[9px] font-medium text-accent border border-accent-edge"
                >
                  {t}
                </span>
              ))}
            </div>
          )}
        </div>
        <span className="text-[11px] text-ink-3">
          {tileUrl ? 'Pan & zoom to stream tiles live' : 'Click geometry for popup details'}
        </span>
      </div>
      <div className="relative h-[460px] w-full">
        <div ref={containerRef} className="absolute inset-0 z-0" />
        {basemapError && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-paper/95 p-6 text-center">
            <div className="max-w-sm space-y-1.5">
              <p className="text-sm font-semibold text-ink">Basemap did not load</p>
              <p className="text-xs text-muted">{basemapError}</p>
              <p className="text-[11px] text-ink-3">
                Check that this browser can reach {STYLE_HOST}. A network filter,
                ad blocker, or offline connection commonly blocks it, and neither
                the basemap nor any feature data can draw until it answers.
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
