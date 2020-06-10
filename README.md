# git-operator

Git Operator is a Kubernetes operator designed to mirror the state of a Git repository as CRD's including:

* Branches 
* Tags
* Pull Requests 
  * Reviewers
  * Comments
  * Checks
  

The operator has the following charecteristics:

  
* Eventual consistency - will poll the repositories periodically to update its state
* Bi-Directional - 
   * Creating a tag CRD object should create the tag in git
   * Deleting a PullRequest should close it
   * Adding comments to a PullRequest via the CRD should reflect in the UI
     
This operator is not meant to be used in isolation but rather as part of a larger workflow where for example a new Pull Request triggers the creation of a Tekton Pipeline run
  
  
  
