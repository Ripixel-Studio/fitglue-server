#!/usr/bin/env npx ts-node

/**
 * backfill-showcase-enrichments.ts
 *
 * Backfills the roundup enrichment-summary fields on every
 * showcase_profile_entry by loading each activity's GCS blob and projecting
 * the relevant enrichments back onto the entry:
 *
 *   - best_efforts        (from enrichments.bestEfforts.efforts)
 *   - primary_muscles     (from enrichments.muscleHeatmap.primary)
 *   - location_name       (from enrichments.location.locationName)
 *   - country             (from enrichments.location.country)
 *   - temp_c              (from enrichments.weather.tempC)
 *   - weather_description (from enrichments.weather.weatherDescription)
 *   - elevation_gain_m    (from enrichments.elevation.totalGainM)
 *
 * Why: these fields are projected onto the entry at showcase time, but that
 * projection only landed on 2026-06-16 (commit f1cf530). Entries showcased
 * before then are missing them, so roundups generated from those entries show
 * empty Best Efforts / Muscles / Places / Weather and zero Elevation — even
 * though the activity's own page shows the data (it hydrates enrichments from
 * GCS at read time). This hydrates the entries so roundup aggregation finds
 * them. Mirrors backfill-showcase-zones.ts.
 *
 * After running this, recompute affected roundups so they pick up the data:
 *   POST /api/v2/users/me/showcase-management/roundup/{periodKey}/recompute
 *
 * Usage:
 *   # Dry run (preview only, no writes):
 *   DRY_RUN=true npx ts-node scripts/backfill-showcase-enrichments.ts
 *
 *   # Single user only:
 *   USER_ID=abc123 npx ts-node scripts/backfill-showcase-enrichments.ts
 *
 *   # All users:
 *   npx ts-node scripts/backfill-showcase-enrichments.ts
 *
 * Auth: uses Application Default Credentials (same as the migration scripts).
 * Run `gcloud auth application-default login` if needed, or set
 * GOOGLE_APPLICATION_CREDENTIALS to a service account key path.
 */

import * as admin from 'firebase-admin';
import { Storage } from '@google-cloud/storage';

const DRY_RUN = process.env.DRY_RUN === 'true';
const TARGET_USER_ID = process.env.USER_ID ?? '';

if (!admin.apps.length) {
    admin.initializeApp();
}
const db = admin.firestore();
const gcs = new Storage();

interface BestEffort {
    distanceKey?: string;
    display?: string;
    distanceM?: number;
    timeSeconds?: number;
}

interface GCSBlob {
    enrichments?: {
        bestEfforts?: { efforts?: BestEffort[] };
        muscleHeatmap?: { primary?: string[] };
        location?: { locationName?: string; country?: string };
        weather?: { tempC?: number; weatherDescription?: string };
        elevation?: { totalGainM?: number };
    };
}

async function downloadBlob(uri: string): Promise<GCSBlob | null> {
    if (!uri.startsWith('gs://')) return null;
    const withoutProtocol = uri.replace('gs://', '');
    const slashIdx = withoutProtocol.indexOf('/');
    if (slashIdx < 0) return null;
    const bucket = withoutProtocol.slice(0, slashIdx);
    const object = withoutProtocol.slice(slashIdx + 1);
    try {
        const [contents] = await gcs.bucket(bucket).file(object).download();
        return JSON.parse(contents.toString('utf8')) as GCSBlob;
    } catch {
        return null;
    }
}

// Mirrors the Go projection in showcase_settings.go (snake_case keys, same guards).
function buildEntryUpdate(blob: GCSBlob): Record<string, unknown> {
    const e = blob.enrichments;
    const update: Record<string, unknown> = {};
    if (!e) return update;

    if (e.elevation && (e.elevation.totalGainM ?? 0) > 0) {
        update.elevation_gain_m = e.elevation.totalGainM;
    }
    if (e.location) {
        if (e.location.locationName) update.location_name = e.location.locationName;
        if (e.location.country) update.country = e.location.country;
    }
    if (e.weather && e.weather.tempC != null) {
        update.temp_c = e.weather.tempC;
        if (e.weather.weatherDescription) update.weather_description = e.weather.weatherDescription;
    }
    const efforts = e.bestEfforts?.efforts ?? [];
    if (efforts.length > 0) {
        update.best_efforts = efforts.map((be) => ({
            distance_key: be.distanceKey ?? '',
            display: be.display ?? '',
            distance_m: be.distanceM ?? 0,
            time_seconds: be.timeSeconds ?? 0,
        }));
    }
    const primary = e.muscleHeatmap?.primary ?? [];
    if (primary.length > 0) update.primary_muscles = primary;

    return update;
}

// True when the entry already carries every summary the blob could provide,
// so we can skip the GCS fetch.
function entryNeedsBackfill(entry: FirebaseFirestore.DocumentData): boolean {
    const hasEfforts = Array.isArray(entry.best_efforts) && entry.best_efforts.length > 0;
    const hasMuscles = Array.isArray(entry.primary_muscles) && entry.primary_muscles.length > 0;
    const hasLocation = !!entry.location_name;
    const hasWeather = entry.temp_c != null;
    const hasElevation = (entry.elevation_gain_m ?? 0) > 0;
    // If none of the summaries are present, it almost certainly predates the
    // projection — fetch and fill. (If some are present we still fetch, since a
    // partial entry can be topped up cheaply at this scale.)
    return !(hasEfforts && hasMuscles && hasLocation && hasWeather && hasElevation);
}

async function backfillUser(userId: string): Promise<{ entries: number; updated: number }> {
    const entriesRef = db.collection('users').doc(userId).collection('showcase_profile_entries');
    const entriesSnap = await entriesRef.get();

    let processed = 0;
    let updated = 0;

    for (const entryDoc of entriesSnap.docs) {
        const entry = entryDoc.data();
        const showcaseId = entry.showcase_id as string | undefined;
        if (!showcaseId) continue;
        processed++;

        if (!entryNeedsBackfill(entry)) continue;

        const activityDoc = await db.collection('showcased_activities').doc(showcaseId).get();
        if (!activityDoc.exists) continue;
        const uri = activityDoc.data()!.activity_data_uri as string | undefined;
        if (!uri) continue;

        const blob = await downloadBlob(uri);
        if (!blob) continue;

        const update = buildEntryUpdate(blob);
        if (Object.keys(update).length === 0) continue;

        updated++;
        const summary = Object.entries(update)
            .map(([k, v]) => `${k}=${Array.isArray(v) ? `[${v.length}]` : v}`)
            .join(', ');
        console.log(`  ${showcaseId}: ${summary}`);
        if (!DRY_RUN) {
            await entriesRef.doc(showcaseId).update(update);
        }
    }

    return { entries: processed, updated };
}

async function main() {
    console.log('='.repeat(60));
    console.log('Showcase Enrichment-Summary Backfill');
    console.log(`Mode: ${DRY_RUN ? 'DRY RUN (no writes)' : 'LIVE'}`);
    if (TARGET_USER_ID) console.log(`User: ${TARGET_USER_ID}`);
    console.log('='.repeat(60));

    const userIds: string[] = [];
    if (TARGET_USER_ID) {
        userIds.push(TARGET_USER_ID);
    } else {
        const slugsSnap = await db.collection('showcase_slugs').get();
        const seen = new Set<string>();
        for (const slugDoc of slugsSnap.docs) {
            const userId = slugDoc.data().user_id as string | undefined;
            if (userId && !seen.has(userId)) {
                seen.add(userId);
                userIds.push(userId);
            }
        }
        console.log(`Found ${userIds.length} users with showcase profiles\n`);
    }

    let totalEntries = 0;
    let totalUpdated = 0;

    for (const userId of userIds) {
        console.log(`\nUser: ${userId}`);
        const { entries, updated } = await backfillUser(userId);
        console.log(`  ${entries} entries processed, ${updated} updated`);
        totalEntries += entries;
        totalUpdated += updated;
    }

    console.log('\n' + '='.repeat(60));
    console.log(`Done. ${totalEntries} entries, ${totalUpdated} updated.`);
    if (DRY_RUN) console.log('DRY RUN — no changes written.');
    console.log('Next: recompute affected roundups via');
    console.log('  POST /api/v2/users/me/showcase-management/roundup/{periodKey}/recompute');
    console.log('='.repeat(60));
}

main().catch(err => { console.error(err); process.exit(1); });
