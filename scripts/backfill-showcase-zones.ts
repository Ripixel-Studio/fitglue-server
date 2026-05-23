#!/usr/bin/env npx ts-node

/**
 * backfill-showcase-zones.ts
 *
 * Backfills hr_zone_minutes on every showcase_profile_entry by loading
 * each activity's GCS blob, extracting HR zone data from the enrichments,
 * and writing it back. Then triggers a zone-split recompute on the profile
 * by writing the aggregated LifetimeZoneSplit directly.
 *
 * Why: the hr_zone_minutes field was added after existing activities were
 * written, so old entries are missing it. recomputeAndSaveProfileStats now
 * uses it, but finds nothing — this script hydrates those entries.
 *
 * Usage:
 *   # Dry run (preview only, no writes):
 *   DRY_RUN=true npx ts-node scripts/backfill-showcase-zones.ts
 *
 *   # Single user only:
 *   USER_ID=abc123 npx ts-node scripts/backfill-showcase-zones.ts
 *
 *   # All users:
 *   npx ts-node scripts/backfill-showcase-zones.ts
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

interface HrZoneBucket {
    zoneIndex?: number;
    name: string;
    minutes?: number;
    percentage?: number;
}

interface HeartRateZonesSummary {
    zones?: HrZoneBucket[];
    totalMinutes?: number;
}

interface GCSBlob {
    enrichments?: {
        heartRateZones?: HeartRateZonesSummary;
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

function buildZoneSplit(zoneMinutes: number[]): {
    zones: object[];
    computed_at: string;
    label: string;
} {
    const total = zoneMinutes.reduce((s, m) => s + m, 0);
    const zoneNames = [
        'Zone 0 (Rest)',
        'Zone 1 (Recovery)',
        'Zone 2 (Endurance)',
        'Zone 3 (Tempo)',
        'Zone 4 (Threshold)',
        'Zone 5 (VO2 Max)',
    ];
    const zones = zoneMinutes.map((mins, i) => ({
        zone_index: i,
        name: zoneNames[i],
        minutes: mins,
        percentage: total > 0 ? (mins / total) * 100 : 0,
    }));
    const z01Pct = total > 0 ? ((zoneMinutes[0] + zoneMinutes[1]) / total) * 100 : 0;
    const z345Pct = total > 0 ? ((zoneMinutes[3] + zoneMinutes[4] + zoneMinutes[5]) / total) * 100 : 0;
    let label = 'Pyramidal';
    if (z01Pct >= 70) label = 'Polarized';
    else if (z345Pct >= 30) label = 'Threshold';
    return { zones, computed_at: new Date().toISOString(), label };
}

async function backfillUser(userId: string): Promise<{ entries: number; withZones: number }> {
    const entriesRef = db.collection('users').doc(userId).collection('showcase_profile_entries');
    const entriesSnap = await entriesRef.get();

    let processed = 0;
    let withZones = 0;
    const profileZoneMinutes = [0, 0, 0, 0, 0, 0];

    for (const entryDoc of entriesSnap.docs) {
        const entry = entryDoc.data();
        const showcaseId = entry.showcase_id as string | undefined;
        if (!showcaseId) continue;
        processed++;

        // If already has zone data, aggregate and skip the GCS fetch.
        if (Array.isArray(entry.hr_zone_minutes) && entry.hr_zone_minutes.some((m: number) => m > 0)) {
            for (let i = 0; i < 6; i++) {
                profileZoneMinutes[i] += (entry.hr_zone_minutes[i] ?? 0);
            }
            withZones++;
            continue;
        }

        // Load the showcased_activity to get the GCS blob URI.
        const activityDoc = await db.collection('showcased_activities').doc(showcaseId).get();
        if (!activityDoc.exists) continue;
        const activityData = activityDoc.data()!;
        const uri = activityData.activity_data_uri as string | undefined;
        if (!uri) continue;

        const blob = await downloadBlob(uri);
        if (!blob?.enrichments?.heartRateZones?.zones?.length) continue;

        const zoneMinutes = [0, 0, 0, 0, 0, 0];
        for (const z of blob.enrichments.heartRateZones.zones) {
            const idx = z.zoneIndex ?? 0;
            if (idx >= 0 && idx < 6) {
                zoneMinutes[idx] = z.minutes ?? 0;
            }
        }

        if (zoneMinutes.every(m => m === 0)) continue;

        withZones++;
        for (let i = 0; i < 6; i++) profileZoneMinutes[i] += zoneMinutes[i];

        if (!DRY_RUN) {
            await entriesRef.doc(showcaseId).update({ hr_zone_minutes: zoneMinutes });
        }
    }

    // Write the aggregated zone split to the showcase profile.
    // Profile lives at users/{userId}/settings/showcase_profile.
    const totalZoneMins = profileZoneMinutes.reduce((s, m) => s + m, 0);
    if (totalZoneMins > 0) {
        const zoneSplit = buildZoneSplit(profileZoneMinutes);
        if (!DRY_RUN) {
            await db
                .collection('users').doc(userId)
                .collection('settings').doc('showcase_profile')
                .set({ zone_split: zoneSplit }, { merge: true });
        }
        console.log(`  → zone split: ${zoneSplit.label} (${profileZoneMinutes.map(m => `${m}m`).join(' / ')})`);
    } else {
        console.log('  → no HR zone data found for this user');
    }

    return { entries: processed, withZones };
}

async function main() {
    console.log('='.repeat(60));
    console.log('Showcase HR Zone Backfill');
    console.log(`Mode: ${DRY_RUN ? 'DRY RUN (no writes)' : 'LIVE'}`);
    if (TARGET_USER_ID) console.log(`User: ${TARGET_USER_ID}`);
    console.log('='.repeat(60));

    const userIds: string[] = [];
    if (TARGET_USER_ID) {
        userIds.push(TARGET_USER_ID);
    } else {
        // showcase_slugs/{slug} → { user_id: string } is the index of all users with a showcase.
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
    let totalWithZones = 0;

    for (const userId of userIds) {
        console.log(`\nUser: ${userId}`);
        const { entries, withZones } = await backfillUser(userId);
        console.log(`  ${entries} entries processed, ${withZones} with HR zone data`);
        totalEntries += entries;
        totalWithZones += withZones;
    }

    console.log('\n' + '='.repeat(60));
    console.log(`Done. ${totalEntries} entries, ${totalWithZones} with zones.`);
    if (DRY_RUN) console.log('DRY RUN — no changes written.');
    console.log('='.repeat(60));
}

main().catch(err => { console.error(err); process.exit(1); });
