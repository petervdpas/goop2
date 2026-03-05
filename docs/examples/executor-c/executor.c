/*
 * Goop2 Cluster Executor — C example
 *
 * Usage: ./executor [goop2-url]
 *        ./executor http://localhost:8787
 *
 * Dependencies: libcurl, cJSON (both widely packaged)
 *
 * Build:
 *   gcc -o executor executor.c -lcurl -lcjson -lpthread
 *   # or with pkg-config:
 *   gcc -o executor executor.c $(pkg-config --cflags --libs libcurl cjson) -lpthread
 *
 * Replace process_job() with your actual workload.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <pthread.h>
#include <curl/curl.h>
#include <cjson/cJSON.h>

#define POLL_INTERVAL_S  2
#define HEARTBEAT_S     10
#define MAX_URL         512
#define MAX_BODY       4096

static char g_base[MAX_URL] = "http://localhost:8787";

/* ── curl helpers ────────────────────────────────────────────────────────── */

typedef struct {
    char  *data;
    size_t len;
} Buffer;

static size_t write_cb(void *ptr, size_t size, size_t nmemb, void *userdata) {
    Buffer *buf = (Buffer *)userdata;
    size_t total = size * nmemb;
    char *tmp = realloc(buf->data, buf->len + total + 1);
    if (!tmp) return 0;
    buf->data = tmp;
    memcpy(buf->data + buf->len, ptr, total);
    buf->len += total;
    buf->data[buf->len] = '\0';
    return total;
}

/* GET request, returns parsed cJSON (caller must cJSON_Delete). */
static cJSON *http_get(const char *url) {
    CURL *c = curl_easy_init();
    if (!c) return NULL;

    Buffer buf = {0};
    curl_easy_setopt(c, CURLOPT_URL, url);
    curl_easy_setopt(c, CURLOPT_WRITEFUNCTION, write_cb);
    curl_easy_setopt(c, CURLOPT_WRITEDATA, &buf);
    curl_easy_setopt(c, CURLOPT_TIMEOUT, 10L);

    CURLcode res = curl_easy_perform(c);
    curl_easy_cleanup(c);

    if (res != CURLE_OK || !buf.data) {
        free(buf.data);
        return NULL;
    }
    cJSON *json = cJSON_Parse(buf.data);
    free(buf.data);
    return json;
}

/* POST JSON, returns HTTP status code (0 on error). */
static int http_post(const char *url, const char *json_body) {
    CURL *c = curl_easy_init();
    if (!c) return 0;

    struct curl_slist *hdrs = NULL;
    hdrs = curl_slist_append(hdrs, "Content-Type: application/json");

    curl_easy_setopt(c, CURLOPT_URL, url);
    curl_easy_setopt(c, CURLOPT_POSTFIELDS, json_body);
    curl_easy_setopt(c, CURLOPT_HTTPHEADER, hdrs);
    curl_easy_setopt(c, CURLOPT_TIMEOUT, 10L);

    CURLcode res = curl_easy_perform(c);
    long status = 0;
    if (res == CURLE_OK)
        curl_easy_getinfo(c, CURLINFO_RESPONSE_CODE, &status);

    curl_slist_free_all(hdrs);
    curl_easy_cleanup(c);
    return (int)status;
}

/* ── API calls ───────────────────────────────────────────────────────────── */

static cJSON *get_jobs(void) {
    char url[MAX_URL];
    snprintf(url, sizeof(url), "%s/api/cluster/job", g_base);
    return http_get(url);
}

static int accept_job(const char *job_id) {
    char url[MAX_URL], body[MAX_BODY];
    snprintf(url, sizeof(url), "%s/api/cluster/accept", g_base);
    snprintf(body, sizeof(body), "{\"job_id\":\"%s\"}", job_id);
    return http_post(url, body);
}

static int report_progress(const char *job_id, int percent, const char *message) {
    char url[MAX_URL], body[MAX_BODY];
    snprintf(url, sizeof(url), "%s/api/cluster/progress", g_base);
    snprintf(body, sizeof(body),
        "{\"job_id\":\"%s\",\"percent\":%d,\"message\":\"%s\"}",
        job_id, percent, message ? message : "");
    return http_post(url, body);
}

static int report_result(const char *job_id, int success,
                         const char *result_json, const char *error_msg) {
    char url[MAX_URL], body[MAX_BODY];
    snprintf(url, sizeof(url), "%s/api/cluster/result", g_base);
    snprintf(body, sizeof(body),
        "{\"job_id\":\"%s\",\"success\":%s,\"result\":%s,\"error\":\"%s\"}",
        job_id,
        success ? "true" : "false",
        result_json ? result_json : "{}",
        error_msg ? error_msg : "");
    return http_post(url, body);
}

static int send_heartbeat(void) {
    char url[MAX_URL], body[MAX_BODY];
    snprintf(url, sizeof(url), "%s/api/cluster/heartbeat", g_base);
    snprintf(body, sizeof(body),
        "{\"stats\":{\"executor\":\"c\",\"pid\":%d}}", (int)getpid());
    return http_post(url, body);
}

/* ── Job processing (replace with your logic) ────────────────────────────── */

static int process_job(const char *job_id, const char *job_type,
                       cJSON *payload, char *result_out, size_t result_sz) {
    int steps = 10;
    cJSON *s = payload ? cJSON_GetObjectItem(payload, "steps") : NULL;
    if (s && cJSON_IsNumber(s)) steps = s->valueint;

    for (int i = 1; i <= steps; i++) {
        usleep(500000); /* 500ms — simulate work */

        int pct = i * 100 / steps;
        char msg[128];
        snprintf(msg, sizeof(msg), "step %d/%d", i, steps);
        report_progress(job_id, pct, msg);
        printf("[executor] progress: %d%% %s\n", pct, msg);
    }

    snprintf(result_out, result_sz,
        "{\"type\":\"%s\",\"steps_completed\":%d}", job_type, steps);
    return 1; /* success */
}

/* ── Heartbeat thread ────────────────────────────────────────────────────── */

static void *heartbeat_thread(void *arg) {
    (void)arg;
    for (;;) {
        send_heartbeat();
        sleep(HEARTBEAT_S);
    }
    return NULL;
}

/* ── Main loop ───────────────────────────────────────────────────────────── */

int main(int argc, char **argv) {
    if (argc > 1)
        snprintf(g_base, sizeof(g_base), "%s", argv[1]);

    curl_global_init(CURL_GLOBAL_DEFAULT);

    pthread_t hb;
    pthread_create(&hb, NULL, heartbeat_thread, NULL);
    pthread_detach(hb);

    printf("[executor] polling %s for jobs...\n", g_base);

    for (;;) {
        cJSON *data = get_jobs();
        if (!data) {
            sleep(POLL_INTERVAL_S);
            continue;
        }

        cJSON *pending = cJSON_GetObjectItem(data, "pending");
        if (!cJSON_IsArray(pending) || cJSON_GetArraySize(pending) == 0) {
            cJSON_Delete(data);
            sleep(POLL_INTERVAL_S);
            continue;
        }

        /* Grab first pending job */
        cJSON *pj  = cJSON_GetArrayItem(pending, 0);
        cJSON *job = cJSON_GetObjectItem(pj, "job");
        const char *job_id   = cJSON_GetStringValue(cJSON_GetObjectItem(job, "id"));
        const char *job_type = cJSON_GetStringValue(cJSON_GetObjectItem(job, "type"));
        cJSON *payload = cJSON_GetObjectItem(job, "payload");

        if (!job_id || !job_type) {
            cJSON_Delete(data);
            sleep(POLL_INTERVAL_S);
            continue;
        }

        printf("[executor] found job %s (type=%s)\n", job_id, job_type);

        int status = accept_job(job_id);
        if (status < 200 || status >= 300) {
            printf("[executor] accept failed (HTTP %d)\n", status);
            cJSON_Delete(data);
            sleep(POLL_INTERVAL_S);
            continue;
        }
        printf("[executor] accepted %s\n", job_id);

        char result_json[MAX_BODY];
        int ok = process_job(job_id, job_type, payload, result_json, sizeof(result_json));

        if (ok) {
            report_result(job_id, 1, result_json, NULL);
            printf("[executor] completed %s\n", job_id);
        } else {
            report_result(job_id, 0, NULL, "processing failed");
            printf("[executor] failed %s\n", job_id);
        }

        cJSON_Delete(data);
    }

    curl_global_cleanup();
    return 0;
}
