import requests
import json
import sys

def test_connectivity():
    """Test API connectivity and diagnose request/response format"""
    base_url = "http://localhost:8000"
    
    # 1. Test health endpoint
    try:
        print("\n=== Testing Health Endpoint ===")
        health_resp = requests.get(f"{base_url}/api/health/ping")
        print(f"Status code: {health_resp.status_code}")
        print(f"Response: {health_resp.text}")
    except Exception as e:
        print(f"Health check error: {str(e)}")
        return False
    
    # 2. Test process endpoint
    print("\n=== Testing Document Process Endpoint ===")
    process_payload = {
        "document_id": "test-doc-id",
        "file_path": "test_file.txt",
        "file_name": "test_file.txt",
        "file_type": "txt",
        "chunk_size": 1000,
        "overlap": 200,
        "split_type": "paragraph",
        "model": "default",
        "metadata": {}
    }
    
    try:
        response = requests.post(f"{base_url}/api/tasks/process", json=process_payload)
        print(f"Status code: {response.status_code}")
        
        try:
            resp_json = response.json()
            print(f"Response: {json.dumps(resp_json, indent=2)}")
            
            # Check for task_id
            if "task_id" in resp_json:
                print(f"Task ID found: {resp_json['task_id']}")
                return True
            else:
                print("No task_id in response")
        except Exception as e:
            print(f"Failed to parse JSON response: {str(e)}")
            print(f"Raw response: {response.text}")
        
    except Exception as e:
        print(f"Error: {str(e)}")
    
    return False

if __name__ == "__main__":
    success = test_connectivity()
    print(f"\nConnectivity test {'successful' if success else 'failed'}")