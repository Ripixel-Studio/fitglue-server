/**
 * Strava Webhook Registration Script
 *
 * This script creates a webhook subscription with Strava for the FitGlue application.
 * It only needs to be run ONCE per environment (dev/prod), not per-user.
 *
 * Usage:
 *   npx ts-node scripts/register-strava-webhook.ts <env>
 *
 *   Where <env> is: dev, test, or prod
 *
 * Prerequisites:
 *   - STRAVA_CLIENT_ID and STRAVA_CLIENT_SECRET must be set in GCP Secret Manager
 *   - STRAVA_VERIFY_TOKEN must be configured for the environment
 *   - The strava-handler Cloud Function must be deployed and accessible
 */

import { SecretManagerServiceClient } from '@google-cloud/secret-manager';

// STRAVA_API_BASE is the Strava REST API base URL.
// Changing to https://www.api-v3.strava.com is required before June 1, 2027.
const STRAVA_API_BASE = 'https://www.strava.com/api/v3';

async function getSecret(projectId: string, secretName: string): Promise<string> {
  const client = new SecretManagerServiceClient();
  const name = `projects/${projectId}/secrets/${secretName}/versions/latest`;
  const [version] = await client.accessSecretVersion({ name });
  return version.payload?.data?.toString() || '';
}

async function main() {
  const env = process.argv[2];

  if (!['dev', 'test', 'prod'].includes(env)) {
    console.error('Usage: npx ts-node scripts/register-strava-webhook.ts <dev|test|prod>');
    process.exit(1);
  }

  const projectId = `fitglue-server-${env}`;

  console.log(`🚀 Registering Strava webhook for ${env} environment`);
  console.log(`📍 Project: ${projectId}`);

  try {
    // Fetch secrets
    const clientId = await getSecret(projectId, 'strava-client-id');
    const clientSecret = await getSecret(projectId, 'strava-client-secret');
    const verifyToken = await getSecret(projectId, 'strava-verify-token');

    if (!clientId || !clientSecret || !verifyToken) {
      console.error('❌ Missing required secrets. Ensure strava-client-id, strava-client-secret, and strava-verify-token are configured.');
      process.exit(1);
    }

    // Construct callback URL based on environment
    // Routes through Firebase Hosting which proxies to Cloud Run
    const domains: Record<string, string> = {
      dev: 'https://dev.fitglue.tech/hooks/strava',
      test: 'https://test.fitglue.tech/hooks/strava',
      prod: 'https://fitglue.tech/hooks/strava'
    };
    const callbackUrl = domains[env];

    console.log(`📡 Callback URL: ${callbackUrl}`);

    // First, check for existing subscriptions
    const listResponse = await fetch(
      `${STRAVA_API_BASE}/push_subscriptions?client_id=${clientId}&client_secret=${clientSecret}`,
      { method: 'GET' }
    );

    if (listResponse.ok) {
      const existingSubs = await listResponse.json() as Array<{ id: number; callback_url: string }>;
      if (existingSubs.length > 0) {
        console.log(`⚠️  Found ${existingSubs.length} existing subscription(s):`);
        existingSubs.forEach(sub => {
          console.log(`   - ID: ${sub.id}, URL: ${sub.callback_url}`);
        });
        console.log('');
        console.log('To update, first delete the existing subscription using:');
        console.log(`  curl -X DELETE "${STRAVA_API_BASE}/push_subscriptions/${existingSubs[0].id}?client_id=${clientId}&client_secret=${clientSecret}"`);
        process.exit(0);
      }
    }

    // Create new subscription
    console.log('📝 Creating new subscription...');

    const createResponse = await fetch(`${STRAVA_API_BASE}/push_subscriptions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        client_id: clientId,
        client_secret: clientSecret,
        callback_url: callbackUrl,
        verify_token: verifyToken
      })
    });

    if (!createResponse.ok) {
      const errorText = await createResponse.text();
      console.error(`❌ Failed to create subscription: ${createResponse.status}`);
      console.error(errorText);
      process.exit(1);
    }

    const result = await createResponse.json() as { id: number };
    console.log(`✅ Subscription created successfully!`);
    console.log(`   Subscription ID: ${result.id}`);
    console.log('');
    console.log('🎉 Strava webhook is now active. All connected users\' activities will be sent to FitGlue.');

  } catch (error) {
    console.error('❌ Error:', error);
    process.exit(1);
  }
}

main();
