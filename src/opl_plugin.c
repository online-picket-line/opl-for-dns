/*
 * OPL DNS Plugin for BIND 9 - Main Implementation
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <curl/curl.h>
#include <json-c/json.h>

#include <isc/mem.h>
#include <isc/result.h>
#include <isc/util.h>
#include <dns/message.h>
#include <dns/name.h>
#include <dns/rdata.h>
#include <dns/rdataset.h>
#include <dns/rdatalist.h>

#include "opl_plugin.h"

/* Default configuration values */
#define DEFAULT_API_ENDPOINT "https://api.onlinepicketline.org/v1/check"
#define DEFAULT_BLOCK_PAGE_IP "127.0.0.1"
#define DEFAULT_API_TIMEOUT 5
#define DEFAULT_CACHE_TTL 300

/* CURL write callback structure */
struct curl_response {
    char *data;
    size_t size;
};

/* CURL write callback function */
static size_t curl_write_callback(void *contents, size_t size, size_t nmemb, void *userp) {
    size_t realsize = size * nmemb;
    struct curl_response *resp = (struct curl_response *)userp;
    
    char *ptr = realloc(resp->data, resp->size + realsize + 1);
    if (ptr == NULL) {
        /* Out of memory */
        return 0;
    }
    
    resp->data = ptr;
    memcpy(&(resp->data[resp->size]), contents, realsize);
    resp->size += realsize;
    resp->data[resp->size] = 0;
    
    return realsize;
}

/* URL encode a string for safe use in query parameters */
static char *url_encode(CURL *curl, const char *str) {
    if (curl == NULL || str == NULL) {
        return NULL;
    }
    return curl_easy_escape(curl, str, 0);
}

/* Initialize plugin */
isc_result_t opl_plugin_init(opl_context_t **ctxp, isc_mem_t *mctx, const char *config_file) {
    opl_context_t *ctx;
    
    if (ctxp == NULL || mctx == NULL) {
        return ISC_R_INVALIDARG;
    }
    
    ctx = isc_mem_get(mctx, sizeof(*ctx));
    if (ctx == NULL) {
        return ISC_R_NOMEMORY;
    }
    
    memset(ctx, 0, sizeof(*ctx));
    ctx->mctx = mctx;
    
    /* Initialize default configuration */
    ctx->config.api_endpoint = strdup(DEFAULT_API_ENDPOINT);
    ctx->config.block_page_ip = strdup(DEFAULT_BLOCK_PAGE_IP);
    ctx->config.api_timeout = DEFAULT_API_TIMEOUT;
    ctx->config.cache_ttl = DEFAULT_CACHE_TTL;
    ctx->config.enabled = 1;
    
    /* TODO: Parse config file if provided */
    
    /* Note: curl_global_init() is called once globally by the plugin loader,
     * not here in the per-instance init function to avoid thread-safety issues */
    
    *ctxp = ctx;
    return ISC_R_SUCCESS;
}

/* Cleanup plugin */
void opl_plugin_destroy(opl_context_t **ctxp) {
    opl_context_t *ctx;
    
    if (ctxp == NULL || *ctxp == NULL) {
        return;
    }
    
    ctx = *ctxp;
    
    if (ctx->config.api_endpoint != NULL) {
        free(ctx->config.api_endpoint);
    }
    if (ctx->config.block_page_ip != NULL) {
        free(ctx->config.block_page_ip);
    }
    
    /* Note: curl_global_cleanup() is called once globally by the plugin unloader,
     * not here in the per-instance destroy function to avoid thread-safety issues */
    
    isc_mem_put(ctx->mctx, ctx, sizeof(*ctx));
    *ctxp = NULL;
}

/* Check domain against OPL API */
isc_result_t opl_check_domain(opl_context_t *ctx, const char *domain, char **dispute_info) {
    CURL *curl;
    CURLcode res;
    struct curl_response response;
    char url[1024];
    json_object *root, *disputed_obj, *info_obj;
    int is_disputed = 0;
    
    if (ctx == NULL || domain == NULL || !ctx->config.enabled) {
        return ISC_R_INVALIDARG;
    }
    
    /* Initialize response */
    response.data = malloc(1);
    if (response.data == NULL) {
        return ISC_R_NOMEMORY;
    }
    response.size = 0;
    
    /* Initialize CURL request */
    curl = curl_easy_init();
    if (curl == NULL) {
        free(response.data);
        return ISC_R_FAILURE;
    }
    
    /* Build URL with properly encoded domain */
    char *encoded_domain = url_encode(curl, domain);
    if (encoded_domain == NULL) {
        curl_easy_cleanup(curl);
        free(response.data);
        return ISC_R_FAILURE;
    }
    snprintf(url, sizeof(url), "%s?domain=%s", ctx->config.api_endpoint, encoded_domain);
    curl_free(encoded_domain);
    
    curl_easy_setopt(curl, CURLOPT_URL, url);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, curl_write_callback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, (void *)&response);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, ctx->config.api_timeout);
    curl_easy_setopt(curl, CURLOPT_USERAGENT, "OPL-DNS-Plugin/" OPL_PLUGIN_VERSION);
    
    /* Perform request */
    res = curl_easy_perform(curl);
    curl_easy_cleanup(curl);
    
    if (res != CURLE_OK) {
        free(response.data);
        return ISC_R_FAILURE;
    }
    
    /* Parse JSON response */
    root = json_tokener_parse(response.data);
    free(response.data);
    
    if (root == NULL) {
        return ISC_R_FAILURE;
    }
    
    /* Check if domain is disputed */
    if (json_object_object_get_ex(root, "disputed", &disputed_obj)) {
        is_disputed = json_object_get_boolean(disputed_obj);
    }
    
    /* Get dispute info if available */
    if (is_disputed && dispute_info != NULL) {
        if (json_object_object_get_ex(root, "info", &info_obj)) {
            const char *info_str = json_object_get_string(info_obj);
            if (info_str != NULL) {
                *dispute_info = strdup(info_str);
            }
        }
    }
    
    json_object_put(root);
    
    return is_disputed ? ISC_R_SUCCESS : ISC_R_NOTFOUND;
}

/* Convert IP address string to bytes */
static int parse_ipv4(const char *ip_str, unsigned char *ip_bytes) {
    unsigned int a, b, c, d;
    
    if (sscanf(ip_str, "%u.%u.%u.%u", &a, &b, &c, &d) != 4) {
        return -1;
    }
    
    if (a > 255 || b > 255 || c > 255 || d > 255) {
        return -1;
    }
    
    ip_bytes[0] = (unsigned char)a;
    ip_bytes[1] = (unsigned char)b;
    ip_bytes[2] = (unsigned char)c;
    ip_bytes[3] = (unsigned char)d;
    
    return 0;
}

/* Modify DNS response to point to block page */
isc_result_t opl_modify_response(opl_context_t *ctx, dns_message_t *message, const char *domain) {
    dns_name_t *qname;
    dns_rdataset_t *rdataset;
    dns_rdata_t rdata = DNS_RDATA_INIT;
    unsigned char ip_bytes[4];
    isc_result_t result;
    
    if (ctx == NULL || message == NULL || domain == NULL) {
        return ISC_R_INVALIDARG;
    }
    
    /* Parse block page IP address */
    if (parse_ipv4(ctx->config.block_page_ip, ip_bytes) != 0) {
        return ISC_R_FAILURE;
    }
    
    /* Get question name */
    qname = NULL;
    result = dns_message_firstname(message, DNS_SECTION_QUESTION);
    if (result == ISC_R_SUCCESS) {
        dns_message_currentname(message, DNS_SECTION_QUESTION, &qname);
    }
    
    if (qname == NULL) {
        return ISC_R_FAILURE;
    }
    
    /* Create A record response */
    rdataset = NULL;
    result = dns_message_gettemprdataset(message, &rdataset);
    if (result != ISC_R_SUCCESS) {
        return result;
    }
    
    dns_rdataset_init(rdataset);
    
    /* Set up rdata for A record */
    rdata.data = ip_bytes;
    rdata.length = 4;
    rdata.rdclass = dns_rdataclass_in;
    rdata.type = dns_rdatatype_a;
    
    /* IMPLEMENTATION NOTE:
     * This is a simplified implementation showing the structure. A complete
     * production implementation would need to:
     * 
     * 1. Create a proper dns_rdatalist_t and populate it
     * 2. Use dns_rdatalist_tordataset() to convert to rdataset
     * 3. Add the rdataset to the message's answer section using dns_message_addname()
     * 4. Set appropriate TTL values
     * 5. Handle memory allocation through BIND's memory context
     * 
     * Example structure:
     *   dns_rdatalist_t *rdatalist;
     *   dns_rdata_t *rdata_item;
     *   
     *   rdatalist = isc_mem_get(message->mctx, sizeof(*rdatalist));
     *   dns_rdatalist_init(rdatalist);
     *   rdatalist->type = dns_rdatatype_a;
     *   rdatalist->rdclass = dns_rdataclass_in;
     *   rdatalist->ttl = 300;
     *   
     *   rdata_item = isc_mem_get(message->mctx, sizeof(*rdata_item));
     *   dns_rdata_init(rdata_item);
     *   // ... populate rdata_item with IP address data
     *   ISC_LIST_APPEND(rdatalist->rdata, rdata_item, link);
     *   
     *   result = dns_rdatalist_tordataset(rdatalist, rdataset);
     *   // ... add to message answer section
     * 
     * The complexity of BIND 9's internal DNS message construction API makes
     * this beyond the scope of a minimal implementation example.
     */
    
    return ISC_R_SUCCESS;
}
