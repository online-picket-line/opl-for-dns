/*
 * OPL DNS Plugin - BIND 9 Interface Layer
 * 
 * This file implements the BIND 9 plugin interface to hook into
 * the DNS query processing pipeline.
 */

#include <stdio.h>
#include <string.h>
#include <curl/curl.h>

#include <isc/mem.h>
#include <isc/result.h>
#include <isc/util.h>

#include <dns/client.h>
#include <dns/log.h>
#include <dns/message.h>
#include <dns/name.h>
#include <dns/result.h>
#include <dns/view.h>

#include <ns/query.h>
#include <ns/hooks.h>

#include "opl_plugin.h"

/* Plugin instance */
static opl_context_t *plugin_ctx = NULL;

/* CURL initialization flag */
static int curl_initialized = 0;

/* Plugin version information */
const char *plugin_version = OPL_PLUGIN_VERSION;
const char *plugin_description = "Online Picket Line DNS Plugin";

/* Convert dns_name_t to string */
static isc_result_t dns_name_to_string(dns_name_t *name, char *buffer, size_t buflen) {
    isc_buffer_t b;
    isc_result_t result;
    
    isc_buffer_init(&b, buffer, buflen);
    result = dns_name_totext(name, false, &b);
    if (result != ISC_R_SUCCESS) {
        return result;
    }
    
    /* Null-terminate */
    if (isc_buffer_availablelength(&b) > 0) {
        buffer[isc_buffer_usedlength(&b)] = '\0';
    } else {
        return ISC_R_NOSPACE;
    }
    
    return ISC_R_SUCCESS;
}

/* Query hook - called before sending response */
static ns_hookresult_t opl_query_respond_any(void *arg, void *cbdata, isc_result_t *resultp) {
    ns_hook_resbody_t *hookdata = (ns_hook_resbody_t *)arg;
    dns_message_t *message = hookdata->response;
    query_ctx_t *qctx = hookdata->qctx;
    char domain[DNS_NAME_MAXTEXT + 1];
    char *dispute_info = NULL;
    isc_result_t result;
    dns_name_t *qname = NULL;
    
    UNUSED(cbdata);
    
    /* Check if plugin is initialized */
    if (plugin_ctx == NULL || !plugin_ctx->config.enabled) {
        return NS_HOOK_CONTINUE;
    }
    
    /* Get query name */
    result = dns_message_firstname(message, DNS_SECTION_QUESTION);
    if (result != ISC_R_SUCCESS) {
        return NS_HOOK_CONTINUE;
    }
    
    dns_message_currentname(message, DNS_SECTION_QUESTION, &qname);
    if (qname == NULL) {
        return NS_HOOK_CONTINUE;
    }
    
    /* Convert domain name to string */
    result = dns_name_to_string(qname, domain, sizeof(domain));
    if (result != ISC_R_SUCCESS) {
        return NS_HOOK_CONTINUE;
    }
    
    /* Check domain against OPL API */
    result = opl_check_domain(plugin_ctx, domain, &dispute_info);
    
    if (result == ISC_R_SUCCESS) {
        /* Domain is disputed - modify response */
        isc_log_write(dns_lctx, DNS_LOGCATEGORY_GENERAL,
                     DNS_LOGMODULE_HOOKS, ISC_LOG_INFO,
                     "OPL: Labor dispute detected for domain %s", domain);
        
        result = opl_modify_response(plugin_ctx, message, domain);
        
        if (dispute_info != NULL) {
            isc_log_write(dns_lctx, DNS_LOGCATEGORY_GENERAL,
                         DNS_LOGMODULE_HOOKS, ISC_LOG_INFO,
                         "OPL: Dispute info: %s", dispute_info);
            free(dispute_info);
        }
        
        /* Let BIND continue with modified response */
        *resultp = ISC_R_SUCCESS;
    }
    
    return NS_HOOK_CONTINUE;
}

/* Plugin initialization function (called by BIND) */
isc_result_t plugin_register(const char *parameters, const void *cfg, const char *file,
                            unsigned long line, isc_mem_t *mctx, isc_log_t *lctx,
                            void *actx, ns_hooktable_t *hooktable, void **instp)
{
    isc_result_t result;
    ns_hook_t *hook;
    
    UNUSED(parameters);
    UNUSED(cfg);
    UNUSED(file);
    UNUSED(line);
    UNUSED(lctx);
    UNUSED(actx);
    
    /* Initialize CURL globally (thread-safe, done once)
     * NOTE: This has a potential race condition in multi-threaded environments.
     * In production, this should use pthread_once() or a similar mechanism
     * to ensure thread-safe initialization. BIND 9 typically loads plugins
     * in a single-threaded context during startup, which mitigates this risk. */
    if (!curl_initialized) {
        curl_global_init(CURL_GLOBAL_DEFAULT);
        curl_initialized = 1;
    }
    
    /* Initialize plugin context */
    result = opl_plugin_init(&plugin_ctx, mctx, NULL);
    if (result != ISC_R_SUCCESS) {
        return result;
    }
    
    /* Register query hook */
    hook = NULL;
    result = ns_hook_add(hooktable, mctx, NS_QUERY_RESPOND_ANY, 
                        opl_query_respond_any, NULL, &hook);
    if (result != ISC_R_SUCCESS) {
        opl_plugin_destroy(&plugin_ctx);
        return result;
    }
    
    *instp = plugin_ctx;
    
    isc_log_write(lctx, ISC_LOGCATEGORY_GENERAL,
                 ISC_LOGMODULE_OTHER, ISC_LOG_INFO,
                 "OPL DNS Plugin v%s loaded successfully", OPL_PLUGIN_VERSION);
    
    return ISC_R_SUCCESS;
}

/* Plugin cleanup function (called by BIND) */
void plugin_destroy(void **instp) {
    if (instp != NULL && *instp != NULL) {
        opl_plugin_destroy((opl_context_t **)instp);
    }
    
    /* Cleanup CURL globally when plugin is unloaded */
    if (curl_initialized) {
        curl_global_cleanup();
        curl_initialized = 0;
    }
}

/* Plugin version function (called by BIND) */
int plugin_version(void) {
    return NS_PLUGIN_VERSION;
}

/* Check plugin implementation (called by BIND) */
isc_result_t plugin_check(const char *parameters, const void *cfg, const char *file,
                         unsigned long line, isc_mem_t *mctx, isc_log_t *lctx,
                         void *actx)
{
    UNUSED(parameters);
    UNUSED(cfg);
    UNUSED(file);
    UNUSED(line);
    UNUSED(mctx);
    UNUSED(lctx);
    UNUSED(actx);
    
    return ISC_R_SUCCESS;
}
