/*
 * Goop2 Cluster Executor — C++ example
 *
 * Usage: ./executor [goop2-url]
 *        ./executor http://localhost:8787
 *
 * Dependencies: libcurl, nlohmann/json (header-only)
 *
 * Build:
 *   g++ -std=c++17 -o executor executor.cpp -lcurl -lpthread
 *
 * nlohmann/json is header-only — either install via package manager
 * (apt install nlohmann-json3-dev) or drop json.hpp next to this file.
 *
 * Replace Executor::processJob() with your actual workload.
 */

#include <iostream>
#include <string>
#include <thread>
#include <chrono>
#include <functional>
#include <curl/curl.h>
#include <nlohmann/json.hpp>

using json = nlohmann::json;
using namespace std::chrono_literals;

// ── HTTP client ─────────────────────────────────────────────────────────────

class HttpClient {
public:
    HttpClient() { curl_global_init(CURL_GLOBAL_DEFAULT); }
    ~HttpClient() { curl_global_cleanup(); }

    json get(const std::string &url) {
        std::string body;
        CURL *c = curl_easy_init();
        curl_easy_setopt(c, CURLOPT_URL, url.c_str());
        curl_easy_setopt(c, CURLOPT_WRITEFUNCTION, writeCb);
        curl_easy_setopt(c, CURLOPT_WRITEDATA, &body);
        curl_easy_setopt(c, CURLOPT_TIMEOUT, 10L);

        CURLcode res = curl_easy_perform(c);
        curl_easy_cleanup(c);

        if (res != CURLE_OK || body.empty())
            return json::object();
        return json::parse(body, nullptr, false);
    }

    int post(const std::string &url, const json &payload) {
        std::string data = payload.dump();
        CURL *c = curl_easy_init();
        struct curl_slist *hdrs = nullptr;
        hdrs = curl_slist_append(hdrs, "Content-Type: application/json");

        curl_easy_setopt(c, CURLOPT_URL, url.c_str());
        curl_easy_setopt(c, CURLOPT_POSTFIELDS, data.c_str());
        curl_easy_setopt(c, CURLOPT_HTTPHEADER, hdrs);
        curl_easy_setopt(c, CURLOPT_TIMEOUT, 10L);

        CURLcode res = curl_easy_perform(c);
        long status = 0;
        if (res == CURLE_OK)
            curl_easy_getinfo(c, CURLINFO_RESPONSE_CODE, &status);

        curl_slist_free_all(hdrs);
        curl_easy_cleanup(c);
        return static_cast<int>(status);
    }

private:
    static size_t writeCb(void *ptr, size_t size, size_t nmemb, void *userdata) {
        auto *buf = static_cast<std::string *>(userdata);
        buf->append(static_cast<char *>(ptr), size * nmemb);
        return size * nmemb;
    }
};

// ── Executor ────────────────────────────────────────────────────────────────

class Executor {
public:
    explicit Executor(std::string baseUrl, int pollMs = 2000)
        : base_(std::move(baseUrl)), pollMs_(pollMs) {}

    // Main loop — blocks forever.
    void run() {
        // Background heartbeat
        std::thread([this]() {
            while (true) {
                heartbeat({{"executor", "cpp"}, {"pid", getpid()}});
                std::this_thread::sleep_for(10s);
            }
        }).detach();

        std::cout << "[executor] polling " << base_ << " for jobs...\n";

        while (true) {
            auto data = http_.get(base_ + "/api/cluster/job");
            if (data.is_discarded() || !data.contains("pending")) {
                std::this_thread::sleep_for(std::chrono::milliseconds(pollMs_));
                continue;
            }

            auto &pending = data["pending"];
            if (!pending.is_array() || pending.empty()) {
                std::this_thread::sleep_for(std::chrono::milliseconds(pollMs_));
                continue;
            }

            auto job = pending[0]["job"];
            std::string jobId   = job["id"];
            std::string jobType = job["type"];
            std::cout << "[executor] found job " << jobId << " (type=" << jobType << ")\n";

            int status = acceptJob(jobId);
            if (status < 200 || status >= 300) {
                std::cerr << "[executor] accept failed (HTTP " << status << ")\n";
                std::this_thread::sleep_for(std::chrono::milliseconds(pollMs_));
                continue;
            }
            std::cout << "[executor] accepted " << jobId << "\n";

            try {
                json result = processJob(jobId, jobType, job.value("payload", json::object()));
                reportResult(jobId, true, result, "");
                std::cout << "[executor] completed " << jobId << "\n";
            } catch (const std::exception &e) {
                reportResult(jobId, false, {}, e.what());
                std::cerr << "[executor] failed " << jobId << ": " << e.what() << "\n";
            }
        }
    }

private:
    // ── Replace this with your actual workload ──────────────────────────

    json processJob(const std::string &jobId, const std::string &jobType,
                    const json &payload) {
        int steps = payload.value("steps", 10);

        for (int i = 1; i <= steps; ++i) {
            std::this_thread::sleep_for(500ms); // simulate work

            int pct = i * 100 / steps;
            std::string msg = "step " + std::to_string(i) + "/" + std::to_string(steps);
            reportProgress(jobId, pct, msg, {{"step", i}});
            std::cout << "[executor] progress: " << pct << "% " << msg << "\n";
        }

        return {{"type", jobType}, {"steps_completed", steps}};
    }

    // ── API calls ───────────────────────────────────────────────────────

    int acceptJob(const std::string &jobId) {
        return http_.post(base_ + "/api/cluster/accept", {{"job_id", jobId}});
    }

    void reportProgress(const std::string &jobId, int percent,
                        const std::string &message, const json &stats) {
        http_.post(base_ + "/api/cluster/progress", {
            {"job_id", jobId}, {"percent", percent},
            {"message", message}, {"stats", stats}
        });
    }

    void reportResult(const std::string &jobId, bool success,
                      const json &result, const std::string &error) {
        http_.post(base_ + "/api/cluster/result", {
            {"job_id", jobId}, {"success", success},
            {"result", result}, {"error", error}
        });
    }

    void heartbeat(const json &stats) {
        http_.post(base_ + "/api/cluster/heartbeat", {{"stats", stats}});
    }

    std::string base_;
    int pollMs_;
    HttpClient http_;
};

// ── main ────────────────────────────────────────────────────────────────────

int main(int argc, char **argv) {
    std::string url = "http://localhost:8787";
    if (argc > 1) url = argv[1];

    Executor(url).run();
    return 0;
}
